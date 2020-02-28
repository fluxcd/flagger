package controller

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
	"github.com/weaveworks/flagger/pkg/metrics/observers"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
	"github.com/weaveworks/flagger/pkg/router"
)

const (
	MetricsProviderServiceSuffix = ":service"
)

// scheduleCanaries synchronises the canary map with the jobs map,
// for new canaries new jobs are created and started
// for the removed canaries the jobs are stopped and deleted
func (c *Controller) scheduleCanaries() {
	current := make(map[string]string)
	stats := make(map[string]int)

	c.canaries.Range(func(key interface{}, value interface{}) bool {
		canary := value.(*flaggerv1.Canary)

		// format: <name>.<namespace>
		name := key.(string)
		current[name] = fmt.Sprintf("%s.%s", canary.Spec.TargetRef.Name, canary.Namespace)

		job, exists := c.jobs[name]
		// schedule new job for existing job with different analysis interval or non-existing job
		if (exists && job.GetCanaryAnalysisInterval() != canary.GetAnalysisInterval()) || !exists {
			if exists {
				job.Stop()
			}

			newJob := CanaryJob{
				Name:             canary.Name,
				Namespace:        canary.Namespace,
				function:         c.advanceCanary,
				done:             make(chan bool),
				ticker:           time.NewTicker(canary.GetAnalysisInterval()),
				analysisInterval: canary.GetAnalysisInterval(),
			}

			c.jobs[name] = newJob
			newJob.Start()
		}

		// compute canaries per namespace total
		t, ok := stats[canary.Namespace]
		if !ok {
			stats[canary.Namespace] = 1
		} else {
			stats[canary.Namespace] = t + 1
		}
		return true
	})

	// cleanup deleted jobs
	for job := range c.jobs {
		if _, exists := current[job]; !exists {
			c.jobs[job].Stop()
			delete(c.jobs, job)
		}
	}

	// check if multiple canaries have the same target
	for canaryName, targetName := range current {
		for name, target := range current {
			if name != canaryName && target == targetName {
				c.logger.With("canary", canaryName).Errorf("Bad things will happen! Found more than one canary with the same target %s", targetName)
			}
		}
	}

	// set total canaries per namespace metric
	for k, v := range stats {
		c.recorder.SetTotal(k, v)
	}
}

func (c *Controller) advanceCanary(name string, namespace string, skipLivenessChecks bool) {
	begin := time.Now()
	// check if the canary exists
	cd, err := c.flaggerClient.FlaggerV1beta1().Canaries(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		c.logger.With("canary", fmt.Sprintf("%s.%s", name, namespace)).
			Errorf("Canary %s.%s not found", name, namespace)
		return
	}

	// override the global provider if one is specified in the canary spec
	provider := c.meshProvider
	if cd.Spec.Provider != "" {
		provider = cd.Spec.Provider
	}

	// init controller based on target kind
	canaryController := c.canaryFactory.Controller(cd.Spec.TargetRef.Kind)
	labelSelector, ports, err := canaryController.GetMetadata(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// init Kubernetes router
	router := c.routerFactory.KubernetesRouter(cd.Spec.TargetRef.Kind, labelSelector, map[string]string{}, ports)
	if err := router.Initialize(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// create primary
	err = canaryController.Initialize(cd, skipLivenessChecks)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// init mesh router
	meshRouter := c.routerFactory.MeshRouter(provider)

	// create or update svc
	if err := router.Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// create or update mesh routes
	if err := meshRouter.Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// check for changes
	shouldAdvance, err := c.shouldAdvance(cd, canaryController)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	if !shouldAdvance {
		c.recorder.SetStatus(cd, cd.Status.Phase)
		return
	}

	// check gates
	if isApproved := c.runConfirmRolloutHooks(cd, canaryController); !isApproved {
		return
	}

	// set max weight default value to 100%
	maxWeight := 100
	if cd.GetAnalysis().MaxWeight > 0 {
		maxWeight = cd.GetAnalysis().MaxWeight
	}

	// check primary status
	if !skipLivenessChecks && !cd.SkipAnalysis() {
		if _, err := canaryController.IsPrimaryReady(cd); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
	}

	// get the routing settings
	primaryWeight, canaryWeight, mirrored, err := meshRouter.GetRoutes(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	c.recorder.SetWeight(cd, primaryWeight, canaryWeight)

	// check if canary analysis should start (canary revision has changes) or continue
	if ok := c.checkCanaryStatus(cd, canaryController, shouldAdvance); !ok {
		return
	}

	// check if canary revision changed during analysis
	if restart := c.hasCanaryRevisionChanged(cd, canaryController); restart {
		c.recordEventInfof(cd, "New revision detected! Restarting analysis for %s.%s",
			cd.Spec.TargetRef.Name, cd.Namespace)

		// route all traffic back to primary
		primaryWeight = 100
		canaryWeight = 0
		if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight, false); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// reset status
		status := flaggerv1.CanaryStatus{
			Phase:        flaggerv1.CanaryPhaseProgressing,
			CanaryWeight: 0,
			FailedChecks: 0,
			Iterations:   0,
		}
		if err := canaryController.SyncStatus(cd, status); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
		return
	}

	// check canary status
	var retriable = true
	if !skipLivenessChecks {
		retriable, err = canaryController.IsCanaryReady(cd)
		if err != nil && retriable {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
	}

	// check if analysis should be skipped
	if skip := c.shouldSkipAnalysis(cd, canaryController, meshRouter, primaryWeight, canaryWeight); skip {
		return
	}

	// check if we should rollback
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseWaiting {
		if ok := c.runRollbackHooks(cd, cd.Status.Phase); ok {
			c.recordEventWarningf(cd, "Rolling back %s.%s manual webhook invoked", cd.Name, cd.Namespace)
			c.alert(cd, "Rolling back manual webhook invoked", false, flaggerv1.SeverityWarn)
			c.rollback(cd, canaryController, meshRouter)
			return
		}
	}

	// route all traffic to primary if analysis has succeeded
	if cd.Status.Phase == flaggerv1.CanaryPhasePromoting {
		if provider != "kubernetes" {
			c.recordEventInfof(cd, "Routing all traffic to primary")
			if err := meshRouter.SetRoutes(cd, 100, 0, false); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recorder.SetWeight(cd, 100, 0)
		}

		// update status phase
		if err := canaryController.SetStatusPhase(cd, flaggerv1.CanaryPhaseFinalising); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		return
	}

	// scale canary to zero if promotion has finished
	if cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		if err := canaryController.Scale(cd, 0); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// set status to succeeded
		if err := canaryController.SetStatusPhase(cd, flaggerv1.CanaryPhaseSucceeded); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseSucceeded)
		c.runPostRolloutHooks(cd, flaggerv1.CanaryPhaseSucceeded)
		c.recordEventInfof(cd, "Promotion completed! Scaling down %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		c.alert(cd, "Canary analysis completed successfully, promotion finished.",
			false, flaggerv1.SeverityInfo)
		return
	}

	// check if the number of failed checks reached the threshold
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing &&
		(!retriable || cd.Status.FailedChecks >= cd.GetAnalysisThreshold()) {
		if !retriable {
			c.recordEventWarningf(cd, "Rolling back %s.%s progress deadline exceeded %v",
				cd.Name, cd.Namespace, err)
			c.alert(cd, fmt.Sprintf("Progress deadline exceeded %v", err),
				false, flaggerv1.SeverityError)
		}
		c.rollback(cd, canaryController, meshRouter)
		return
	}

	// record analysis duration
	defer func() {
		c.recorder.SetDuration(cd, time.Since(begin))
	}()

	// check if the canary success rate is above the threshold
	// skip check if no traffic is routed or mirrored to canary
	if canaryWeight == 0 && cd.Status.Iterations == 0 &&
		(cd.GetAnalysis().Mirror == false || mirrored == false) {
		c.recordEventInfof(cd, "Starting canary analysis for %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)

		// run pre-rollout web hooks
		if ok := c.runPreRolloutHooks(cd); !ok {
			if err := canaryController.SetStatusFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}
	} else {
		if ok := c.runAnalysis(cd); !ok {
			if err := canaryController.SetStatusFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}
	}

	// use blue/green strategy for kubernetes provider
	if provider == "kubernetes" {
		if len(cd.GetAnalysis().Match) > 0 {
			c.recordEventWarningf(cd, "A/B testing is not supported when using the kubernetes provider")
			cd.GetAnalysis().Match = nil
		}
		if cd.GetAnalysis().Iterations < 1 {
			c.recordEventWarningf(cd, "Progressive traffic is not supported when using the kubernetes provider")
			c.recordEventWarningf(cd, "Setting canaryAnalysis.iterations: 10")
			cd.GetAnalysis().Iterations = 10
		}
	}

	// strategy: A/B testing
	if len(cd.GetAnalysis().Match) > 0 && cd.GetAnalysis().Iterations > 0 {
		c.runAB(cd, canaryController, meshRouter, provider)
		return
	}

	// strategy: Blue/Green
	if cd.GetAnalysis().Iterations > 0 {
		c.runBlueGreen(cd, canaryController, meshRouter, provider, mirrored)
		return
	}

	// strategy: Canary progressive traffic increase
	if cd.GetAnalysis().StepWeight > 0 {
		c.runCanary(cd, canaryController, meshRouter, provider, mirrored, canaryWeight, primaryWeight, maxWeight)
	}

}

func (c *Controller) runCanary(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface, provider string, mirrored bool, canaryWeight int, primaryWeight int, maxWeight int) {
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	// increase traffic weight
	if canaryWeight < maxWeight {
		// If in "mirror" mode, do one step of mirroring before shifting traffic to canary.
		// When mirroring, all requests go to primary and canary, but only responses from
		// primary go back to the user.
		if canary.GetAnalysis().Mirror && canaryWeight == 0 {
			if mirrored == false {
				mirrored = true
				primaryWeight = 100
				canaryWeight = 0
			} else {
				mirrored = false
				primaryWeight = 100 - canary.GetAnalysis().StepWeight
				canaryWeight = canary.GetAnalysis().StepWeight
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Running mirror step %d/%d/%t", primaryWeight, canaryWeight, mirrored)
		} else {

			primaryWeight -= canary.GetAnalysis().StepWeight
			if primaryWeight < 0 {
				primaryWeight = 0
			}
			canaryWeight += canary.GetAnalysis().StepWeight
			if canaryWeight > 100 {
				canaryWeight = 100
			}
		}

		if err := meshRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}

		if err := canaryController.SetStatusWeight(canary, canaryWeight); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}

		c.recorder.SetWeight(canary, primaryWeight, canaryWeight)
		c.recordEventInfof(canary, "Advance %s.%s canary weight %v", canary.Name, canary.Namespace, canaryWeight)
		return
	}

	// promote canary - max weight reached
	if canaryWeight >= maxWeight {
		// check promotion gate
		if promote := c.runConfirmPromotionHooks(canary); !promote {
			return
		}

		// update primary spec
		c.recordEventInfof(canary, "Copying %s.%s template spec to %s.%s",
			canary.Spec.TargetRef.Name, canary.Namespace, primaryName, canary.Namespace)
		if err := canaryController.Promote(canary); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}

		// update status phase
		if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhasePromoting); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
	}
}

func (c *Controller) runAB(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface, provider string) {
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	// route traffic to canary and increment iterations
	if canary.GetAnalysis().Iterations > canary.Status.Iterations {
		if err := meshRouter.SetRoutes(canary, 0, 100, false); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recorder.SetWeight(canary, 0, 100)

		if err := canaryController.SetStatusIterations(canary, canary.Status.Iterations+1); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recordEventInfof(canary, "Advance %s.%s canary iteration %v/%v",
			canary.Name, canary.Namespace, canary.Status.Iterations+1, canary.GetAnalysis().Iterations)
		return
	}

	// check promotion gate
	if promote := c.runConfirmPromotionHooks(canary); !promote {
		return
	}

	// promote canary - max iterations reached
	if canary.GetAnalysis().Iterations == canary.Status.Iterations {
		c.recordEventInfof(canary, "Copying %s.%s template spec to %s.%s",
			canary.Spec.TargetRef.Name, canary.Namespace, primaryName, canary.Namespace)
		if err := canaryController.Promote(canary); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}

		// update status phase
		if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhasePromoting); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
	}
}

func (c *Controller) runBlueGreen(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface, provider string, mirrored bool) {
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	// increment iterations
	if canary.GetAnalysis().Iterations > canary.Status.Iterations {
		// If in "mirror" mode, mirror requests during the entire B/G canary test
		if provider != "kubernetes" &&
			canary.GetAnalysis().Mirror == true && mirrored == false {
			if err := meshRouter.SetRoutes(canary, 100, 0, true); err != nil {
				c.recordEventWarningf(canary, "%v", err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Start traffic mirroring")
		}
		if err := canaryController.SetStatusIterations(canary, canary.Status.Iterations+1); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recordEventInfof(canary, "Advance %s.%s canary iteration %v/%v",
			canary.Name, canary.Namespace, canary.Status.Iterations+1, canary.GetAnalysis().Iterations)
		return
	}

	// check promotion gate
	if promote := c.runConfirmPromotionHooks(canary); !promote {
		return
	}

	// route all traffic to canary - max iterations reached
	if canary.GetAnalysis().Iterations == canary.Status.Iterations {
		if provider != "kubernetes" {
			if canary.GetAnalysis().Mirror {
				c.recordEventInfof(canary, "Stop traffic mirroring and route all traffic to canary")
			} else {
				c.recordEventInfof(canary, "Routing all traffic to canary")
			}
			if err := meshRouter.SetRoutes(canary, 0, 100, false); err != nil {
				c.recordEventWarningf(canary, "%v", err)
				return
			}
			c.recorder.SetWeight(canary, 0, 100)
		}

		// increment iterations
		if err := canaryController.SetStatusIterations(canary, canary.Status.Iterations+1); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		return
	}

	// promote canary - max iterations reached
	if canary.GetAnalysis().Iterations < canary.Status.Iterations {
		c.recordEventInfof(canary, "Copying %s.%s template spec to %s.%s",
			canary.Spec.TargetRef.Name, canary.Namespace, primaryName, canary.Namespace)
		if err := canaryController.Promote(canary); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}

		// update status phase
		if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhasePromoting); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
	}

}

func (c *Controller) shouldSkipAnalysis(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface, primaryWeight int, canaryWeight int) bool {
	if !canary.SkipAnalysis() {
		return false
	}

	// route all traffic to primary
	primaryWeight = 100
	canaryWeight = 0
	if err := meshRouter.SetRoutes(canary, primaryWeight, canaryWeight, false); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return false
	}
	c.recorder.SetWeight(canary, primaryWeight, canaryWeight)

	// copy spec and configs from canary to primary
	c.recordEventInfof(canary, "Copying %s.%s template spec to %s-primary.%s",
		canary.Spec.TargetRef.Name, canary.Namespace, canary.Spec.TargetRef.Name, canary.Namespace)
	if err := canaryController.Promote(canary); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return false
	}

	// shutdown canary
	if err := canaryController.Scale(canary, 0); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return false
	}

	// update status phase
	if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseSucceeded); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return false
	}

	// notify
	c.recorder.SetStatus(canary, flaggerv1.CanaryPhaseSucceeded)
	c.recordEventInfof(canary, "Promotion completed! Canary analysis was skipped for %s.%s",
		canary.Spec.TargetRef.Name, canary.Namespace)
	c.alert(canary, "Canary analysis was skipped, promotion finished.",
		false, flaggerv1.SeverityInfo)

	return true
}

func (c *Controller) shouldAdvance(canary *flaggerv1.Canary, canaryController canary.Controller) (bool, error) {
	if canary.Status.LastAppliedSpec == "" ||
		canary.Status.Phase == flaggerv1.CanaryPhaseInitializing ||
		canary.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		canary.Status.Phase == flaggerv1.CanaryPhaseWaiting ||
		canary.Status.Phase == flaggerv1.CanaryPhasePromoting ||
		canary.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true, nil
	}

	newTarget, err := canaryController.HasTargetChanged(canary)
	if err != nil {
		return false, err
	}
	if newTarget {
		return newTarget, nil
	}

	newCfg, err := canaryController.HaveDependenciesChanged(canary)
	if err != nil {
		return false, err
	}

	return newCfg, nil

}

func (c *Controller) checkCanaryStatus(canary *flaggerv1.Canary, canaryController canary.Controller, shouldAdvance bool) bool {
	c.recorder.SetStatus(canary, canary.Status.Phase)
	if canary.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		canary.Status.Phase == flaggerv1.CanaryPhasePromoting ||
		canary.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true
	}

	if canary.Status.Phase == "" || canary.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if err := canaryController.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseInitialized}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(canary, flaggerv1.CanaryPhaseInitialized)
		c.recordEventInfof(canary, "Initialization done! %s.%s", canary.Name, canary.Namespace)
		c.alert(canary, fmt.Sprintf("New %s detected, initialization completed.", canary.Spec.TargetRef.Kind),
			true, flaggerv1.SeverityInfo)
		return false
	}

	if shouldAdvance {
		canaryPhaseProgressing := canary.DeepCopy()
		canaryPhaseProgressing.Status.Phase = flaggerv1.CanaryPhaseProgressing
		c.recordEventInfof(canaryPhaseProgressing, "New revision detected! Scaling up %s.%s", canaryPhaseProgressing.Spec.TargetRef.Name, canaryPhaseProgressing.Namespace)
		c.alert(canaryPhaseProgressing, "New revision detected, starting canary analysis.",
			true, flaggerv1.SeverityInfo)

		if err := canaryController.ScaleFromZero(canary); err != nil {
			c.recordEventErrorf(canary, "%v", err)
			return false
		}
		if err := canaryController.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(canary, flaggerv1.CanaryPhaseProgressing)
		return false
	}
	return false
}

func (c *Controller) hasCanaryRevisionChanged(canary *flaggerv1.Canary, canaryController canary.Controller) bool {
	if canary.Status.Phase == flaggerv1.CanaryPhaseProgressing {
		if diff, _ := canaryController.HasTargetChanged(canary); diff {
			return true
		}
		if diff, _ := canaryController.HaveDependenciesChanged(canary); diff {
			return true
		}
	}
	return false
}

func (c *Controller) runConfirmRolloutHooks(canary *flaggerv1.Canary, canaryController canary.Controller) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmRolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				if canary.Status.Phase != flaggerv1.CanaryPhaseWaiting {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseWaiting); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
					}
					c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for approval %s",
						canary.Name, canary.Namespace, webhook.Name)
					c.alert(canary, "Canary is waiting for approval.", false, flaggerv1.SeverityWarn)
				}
				return false
			} else {
				if canary.Status.Phase == flaggerv1.CanaryPhaseWaiting {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseProgressing); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
						return false
					}
					c.recordEventInfof(canary, "Confirm-rollout check %s passed", webhook.Name)
					return false
				}
			}
		}
	}
	return true
}

func (c *Controller) runConfirmPromotionHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmPromotionHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for promotion approval %s",
					canary.Name, canary.Namespace, webhook.Name)
				c.alert(canary, "Canary promotion is waiting for approval.", false, flaggerv1.SeverityWarn)
				return false
			} else {
				c.recordEventInfof(canary, "Confirm-promotion check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runPreRolloutHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.PreRolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement pre-rollout check %s failed %v",
					canary.Name, canary.Namespace, webhook.Name, err)
				return false
			} else {
				c.recordEventInfof(canary, "Pre-rollout check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runPostRolloutHooks(canary *flaggerv1.Canary, phase flaggerv1.CanaryPhase) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.PostRolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, phase, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Post-rollout hook %s failed %v", webhook.Name, err)
				return false
			} else {
				c.recordEventInfof(canary, "Post-rollout check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runRollbackHooks(canary *flaggerv1.Canary, phase flaggerv1.CanaryPhase) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.RollbackHook {
			err := CallWebhook(canary.Name, canary.Namespace, phase, webhook)
			if err != nil {
				c.recordEventInfof(canary, "Rollback hook %s not signaling a rollback", webhook.Name)
			} else {
				c.recordEventWarningf(canary, "Rollback check %s passed", webhook.Name)
				return true
			}
		}
	}
	return false
}

func (c *Controller) runAnalysis(canary *flaggerv1.Canary) bool {
	// run external checks
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == "" || webhook.Type == flaggerv1.RolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement external check %s failed %v",
					canary.Name, canary.Namespace, webhook.Name, err)
				return false
			}
		}
	}

	ok := c.runBuiltinMetricChecks(canary)
	if !ok {
		return ok
	}

	ok = c.runMetricChecks(canary)
	if !ok {
		return ok
	}

	return true
}

func (c *Controller) runBuiltinMetricChecks(canary *flaggerv1.Canary) bool {
	// override the global provider if one is specified in the canary spec
	var metricsProvider string
	// set the metrics provider to Crossover Prometheus when Crossover is the mesh provider
	// For example, `crossover` metrics provider should be used for `smi:crossover` mesh provider
	if strings.Contains(c.meshProvider, "crossover") {
		metricsProvider = "crossover"
	} else {
		metricsProvider = c.meshProvider
	}

	if canary.Spec.Provider != "" {
		metricsProvider = canary.Spec.Provider

		// set the metrics provider to Linkerd Prometheus when Linkerd is the default mesh provider
		if strings.Contains(c.meshProvider, "linkerd") {
			metricsProvider = "linkerd"
		}
	}
	// set the metrics provider to query Prometheus for the canary Kubernetes service if the canary target is Service
	if canary.Spec.TargetRef.Kind == "Service" {
		metricsProvider = metricsProvider + MetricsProviderServiceSuffix
	}

	// create observer based on the mesh provider
	observerFactory := c.observerFactory

	// override the global metrics server if one is specified in the canary spec
	if canary.Spec.MetricsServer != "" {
		var err error
		observerFactory, err = observers.NewFactory(canary.Spec.MetricsServer)
		if err != nil {
			c.recordEventErrorf(canary, "Error building Prometheus client for %s %v", canary.Spec.MetricsServer, err)
			return false
		}
	}
	observer := observerFactory.Observer(metricsProvider)

	// run metrics checks
	for _, metric := range canary.GetAnalysis().Metrics {
		if metric.Interval == "" {
			metric.Interval = canary.GetMetricInterval()
		}

		if metric.Name == "request-success-rate" {
			val, err := observer.GetRequestSuccessRate(toMetricModel(canary, metric.Interval))
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(canary, "Halt advancement no values found for %s metric %s probably %s.%s is not receiving traffic",
						metricsProvider, metric.Name, canary.Spec.TargetRef.Name, canary.Namespace)
				} else {
					c.recordEventErrorf(canary, "Prometheus query failed: %v", err)
				}
				return false
			}

			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < *tr.Min {
					c.recordEventWarningf(canary, "Halt %s.%s advancement success rate %.2f%% < %v%%",
						canary.Name, canary.Namespace, val, *tr.Min)
					return false
				}
				if tr.Max != nil && val > *tr.Max {
					c.recordEventWarningf(canary, "Halt %s.%s advancement success rate %.2f%% > %v%%",
						canary.Name, canary.Namespace, val, *tr.Max)
					return false
				}
			} else if metric.Threshold > val {
				c.recordEventWarningf(canary, "Halt %s.%s advancement success rate %.2f%% < %v%%",
					canary.Name, canary.Namespace, val, metric.Threshold)
				return false
			}
		}

		if metric.Name == "request-duration" {
			val, err := observer.GetRequestDuration(toMetricModel(canary, metric.Interval))
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(canary, "Halt advancement no values found for %s metric %s probably %s.%s is not receiving traffic",
						metricsProvider, metric.Name, canary.Spec.TargetRef.Name, canary.Namespace)
				} else {
					c.recordEventErrorf(canary, "Prometheus query failed: %v", err)
				}
				return false
			}
			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < time.Duration(*tr.Min)*time.Millisecond {
					c.recordEventWarningf(canary, "Halt %s.%s advancement request duration %v < %v",
						canary.Name, canary.Namespace, val, time.Duration(*tr.Min)*time.Millisecond)
					return false
				}
				if tr.Max != nil && val > time.Duration(*tr.Max)*time.Millisecond {
					c.recordEventWarningf(canary, "Halt %s.%s advancement request duration %v > %v",
						canary.Name, canary.Namespace, val, time.Duration(*tr.Max)*time.Millisecond)
					return false
				}
			} else if val > time.Duration(metric.Threshold)*time.Millisecond {
				c.recordEventWarningf(canary, "Halt %s.%s advancement request duration %v > %v",
					canary.Name, canary.Namespace, val, time.Duration(metric.Threshold)*time.Millisecond)
				return false
			}
		}

		// in-line PromQL
		if metric.Query != "" {
			val, err := observerFactory.Client.RunQuery(metric.Query)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(canary, "Halt advancement no values found for metric: %s",
						metric.Name)
				} else {
					c.recordEventErrorf(canary, "Prometheus query failed for %s: %v", metric.Name, err)
				}
				return false
			}
			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < *tr.Min {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f < %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Min)
					return false
				}
				if tr.Max != nil && val > *tr.Max {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Max)
					return false
				}
			} else if val > metric.Threshold {
				c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
					canary.Name, canary.Namespace, metric.Name, val, metric.Threshold)
				return false
			}
		}
	}

	return true
}

func (c *Controller) runMetricChecks(canary *flaggerv1.Canary) bool {
	for _, metric := range canary.GetAnalysis().Metrics {
		if metric.TemplateRef != nil {
			namespace := canary.Namespace
			if metric.TemplateRef.Namespace != "" {
				namespace = metric.TemplateRef.Namespace
			}

			template, err := c.flaggerInformers.MetricInformer.Lister().MetricTemplates(namespace).Get(metric.TemplateRef.Name)
			if err != nil {
				c.recordEventErrorf(canary, "Metric template %s.%s error: %v", metric.TemplateRef.Name, namespace, err)
				return false
			}

			var credentials map[string][]byte
			if template.Spec.Provider.SecretRef != nil {
				secret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(template.Spec.Provider.SecretRef.Name, metav1.GetOptions{})
				if err != nil {
					c.recordEventErrorf(canary, "Metric template %s.%s secret %s error: %v",
						metric.TemplateRef.Name, namespace, template.Spec.Provider.SecretRef.Name, err)
					return false
				}
				credentials = secret.Data
			}

			factory := providers.Factory{}
			provider, err := factory.Provider(metric.Interval, template.Spec.Provider, credentials)
			if err != nil {
				c.recordEventErrorf(canary, "Metric template %s.%s provider %s error: %v",
					metric.TemplateRef.Name, namespace, template.Spec.Provider.Type, err)
				return false
			}

			query, err := observers.RenderQuery(template.Spec.Query, toMetricModel(canary, metric.Interval))
			if err != nil {
				c.recordEventErrorf(canary, "Metric template %s.%s query render error: %v",
					metric.TemplateRef.Name, namespace, err)
				return false
			}

			val, err := provider.RunQuery(query)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(canary, "Halt advancement no values found for custom metric: %s",
						metric.Name)
				} else {
					c.recordEventErrorf(canary, "Metric query failed for %s: %v", metric.Name, err)
				}
				return false
			}

			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < *tr.Min {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f < %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Min)
					return false
				}
				if tr.Max != nil && val > *tr.Max {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Max)
					return false
				}
			} else if val > metric.Threshold {
				c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
					canary.Name, canary.Namespace, metric.Name, val, metric.Threshold)
				return false
			}
		}
	}

	return true
}

func toMetricModel(r *flaggerv1.Canary, interval string) flaggerv1.MetricTemplateModel {
	service := r.Spec.TargetRef.Name
	if r.Spec.Service.Name != "" {
		service = r.Spec.Service.Name
	}
	ingress := r.Spec.TargetRef.Name
	if r.Spec.IngressRef != nil {
		ingress = r.Spec.IngressRef.Name
	}
	return flaggerv1.MetricTemplateModel{
		Name:      r.Name,
		Namespace: r.Namespace,
		Target:    r.Spec.TargetRef.Name,
		Service:   service,
		Ingress:   ingress,
		Interval:  interval,
	}
}

func (c *Controller) rollback(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface) {
	if canary.Status.FailedChecks >= canary.GetAnalysisThreshold() {
		c.recordEventWarningf(canary, "Rolling back %s.%s failed checks threshold reached %v",
			canary.Name, canary.Namespace, canary.Status.FailedChecks)
		c.alert(canary, fmt.Sprintf("Failed checks threshold reached %v", canary.Status.FailedChecks),
			false, flaggerv1.SeverityError)
	}

	// route all traffic back to primary
	primaryWeight := 100
	canaryWeight := 0
	if err := meshRouter.SetRoutes(canary, primaryWeight, canaryWeight, false); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return
	}

	canaryPhaseFailed := canary.DeepCopy()
	canaryPhaseFailed.Status.Phase = flaggerv1.CanaryPhaseFailed
	c.recordEventWarningf(canaryPhaseFailed, "Canary failed! Scaling down %s.%s",
		canaryPhaseFailed.Name, canaryPhaseFailed.Namespace)

	c.recorder.SetWeight(canary, primaryWeight, canaryWeight)

	// shutdown canary
	if err := canaryController.Scale(canary, 0); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return
	}

	// mark canary as failed
	if err := canaryController.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseFailed, CanaryWeight: 0}); err != nil {
		c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
		return
	}

	c.recorder.SetStatus(canary, flaggerv1.CanaryPhaseFailed)
	c.runPostRolloutHooks(canary, flaggerv1.CanaryPhaseFailed)
}
