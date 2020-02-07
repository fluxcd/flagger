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

	// create primary deployment and hpa
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
	if cd.Spec.CanaryAnalysis.MaxWeight > 0 {
		maxWeight = cd.Spec.CanaryAnalysis.MaxWeight
	}

	// check primary deployment status
	if !skipLivenessChecks && !cd.Spec.SkipAnalysis {
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

	// check canary deployment status
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
			c.sendNotification(cd, "Rolling back manual webhook invoked", false, true)
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
		c.sendNotification(cd, "Canary analysis completed successfully, promotion finished.",
			false, false)
		return
	}

	// check if the number of failed checks reached the threshold
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing &&
		(!retriable || cd.Status.FailedChecks >= cd.Spec.CanaryAnalysis.Threshold) {
		if !retriable {
			c.recordEventWarningf(cd, "Rolling back %s.%s progress deadline exceeded %v",
				cd.Name, cd.Namespace, err)
			c.sendNotification(cd, fmt.Sprintf("Progress deadline exceeded %v", err),
				false, true)
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
		(cd.Spec.CanaryAnalysis.Mirror == false || mirrored == false) {
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
		if len(cd.Spec.CanaryAnalysis.Match) > 0 {
			c.recordEventWarningf(cd, "A/B testing is not supported when using the kubernetes provider")
			cd.Spec.CanaryAnalysis.Match = nil
		}
		if cd.Spec.CanaryAnalysis.Iterations < 1 {
			c.recordEventWarningf(cd, "Progressive traffic is not supported when using the kubernetes provider")
			c.recordEventWarningf(cd, "Setting canaryAnalysis.iterations: 10")
			cd.Spec.CanaryAnalysis.Iterations = 10
		}
	}

	// strategy: A/B testing
	if len(cd.Spec.CanaryAnalysis.Match) > 0 && cd.Spec.CanaryAnalysis.Iterations > 0 {
		c.runAB(cd, canaryController, meshRouter, provider)
		return
	}

	// strategy: Blue/Green
	if cd.Spec.CanaryAnalysis.Iterations > 0 {
		c.runBlueGreen(cd, canaryController, meshRouter, provider, mirrored)
		return
	}

	// strategy: Canary progressive traffic increase
	if cd.Spec.CanaryAnalysis.StepWeight > 0 {
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
		if canary.Spec.CanaryAnalysis.Mirror && canaryWeight == 0 {
			if mirrored == false {
				mirrored = true
				primaryWeight = 100
				canaryWeight = 0
			} else {
				mirrored = false
				primaryWeight = 100 - canary.Spec.CanaryAnalysis.StepWeight
				canaryWeight = canary.Spec.CanaryAnalysis.StepWeight
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Running mirror step %d/%d/%t", primaryWeight, canaryWeight, mirrored)
		} else {

			primaryWeight -= canary.Spec.CanaryAnalysis.StepWeight
			if primaryWeight < 0 {
				primaryWeight = 0
			}
			canaryWeight += canary.Spec.CanaryAnalysis.StepWeight
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
	if canary.Spec.CanaryAnalysis.Iterations > canary.Status.Iterations {
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
			canary.Name, canary.Namespace, canary.Status.Iterations+1, canary.Spec.CanaryAnalysis.Iterations)
		return
	}

	// check promotion gate
	if promote := c.runConfirmPromotionHooks(canary); !promote {
		return
	}

	// promote canary - max iterations reached
	if canary.Spec.CanaryAnalysis.Iterations == canary.Status.Iterations {
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
	if canary.Spec.CanaryAnalysis.Iterations > canary.Status.Iterations {
		// If in "mirror" mode, mirror requests during the entire B/G canary test
		if provider != "kubernetes" &&
			canary.Spec.CanaryAnalysis.Mirror == true && mirrored == false {
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
			canary.Name, canary.Namespace, canary.Status.Iterations+1, canary.Spec.CanaryAnalysis.Iterations)
		return
	}

	// check promotion gate
	if promote := c.runConfirmPromotionHooks(canary); !promote {
		return
	}

	// route all traffic to canary - max iterations reached
	if canary.Spec.CanaryAnalysis.Iterations == canary.Status.Iterations {
		if provider != "kubernetes" {
			if canary.Spec.CanaryAnalysis.Mirror {
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
	if canary.Spec.CanaryAnalysis.Iterations < canary.Status.Iterations {
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
	if !canary.Spec.SkipAnalysis {
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
	c.sendNotification(canary, "Canary analysis was skipped, promotion finished.",
		false, false)

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
		c.sendNotification(canary, "New deployment detected, initialization completed.",
			true, false)
		return false
	}

	if shouldAdvance {
		canaryPhaseProgressing := canary.DeepCopy()
		canaryPhaseProgressing.Status.Phase = flaggerv1.CanaryPhaseProgressing
		c.recordEventInfof(canaryPhaseProgressing, "New revision detected! Scaling up %s.%s", canaryPhaseProgressing.Spec.TargetRef.Name, canaryPhaseProgressing.Namespace)
		c.sendNotification(canaryPhaseProgressing, "New revision detected, starting canary analysis.",
			true, false)

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
	for _, webhook := range canary.Spec.CanaryAnalysis.Webhooks {
		if webhook.Type == flaggerv1.ConfirmRolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				if canary.Status.Phase != flaggerv1.CanaryPhaseWaiting {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseWaiting); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
					}
					c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for approval %s",
						canary.Name, canary.Namespace, webhook.Name)
					c.sendNotification(canary, "Canary is waiting for approval.", false, false)
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
	for _, webhook := range canary.Spec.CanaryAnalysis.Webhooks {
		if webhook.Type == flaggerv1.ConfirmPromotionHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for promotion approval %s",
					canary.Name, canary.Namespace, webhook.Name)
				c.sendNotification(canary, "Canary promotion is waiting for approval.", false, false)
				return false
			} else {
				c.recordEventInfof(canary, "Confirm-promotion check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runPreRolloutHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.Spec.CanaryAnalysis.Webhooks {
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
	for _, webhook := range canary.Spec.CanaryAnalysis.Webhooks {
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
	for _, webhook := range canary.Spec.CanaryAnalysis.Webhooks {
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

func (c *Controller) runAnalysis(r *flaggerv1.Canary) bool {
	// run external checks
	for _, webhook := range r.Spec.CanaryAnalysis.Webhooks {
		if webhook.Type == "" || webhook.Type == flaggerv1.RolloutHook {
			err := CallWebhook(r.Name, r.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(r, "Halt %s.%s advancement external check %s failed %v",
					r.Name, r.Namespace, webhook.Name, err)
				return false
			}
		}
	}

	ok := c.runBuiltinMetricChecks(r)
	if !ok {
		return ok
	}

	ok = c.runMetricChecks(r)
	if !ok {
		return ok
	}

	return true
}

func (c *Controller) runBuiltinMetricChecks(r *flaggerv1.Canary) bool {
	// override the global provider if one is specified in the canary spec
	var metricsProvider string
	// set the metrics provider to Crossover Prometheus when Crossover is the mesh provider
	// For example, `crossover` metrics provider should be used for `smi:crossover` mesh provider
	if strings.Contains(c.meshProvider, "crossover") {
		metricsProvider = "crossover"
	} else {
		metricsProvider = c.meshProvider
	}

	if r.Spec.Provider != "" {
		metricsProvider = r.Spec.Provider

		// set the metrics provider to Linkerd Prometheus when Linkerd is the default mesh provider
		if strings.Contains(c.meshProvider, "linkerd") {
			metricsProvider = "linkerd"
		}
	}
	// set the metrics provider to query Prometheus for the canary Kubernetes service if the canary target is Service
	if r.Spec.TargetRef.Kind == "Service" {
		metricsProvider = metricsProvider + MetricsProviderServiceSuffix
	}

	// create observer based on the mesh provider
	observerFactory := c.observerFactory

	// override the global metrics server if one is specified in the canary spec
	if r.Spec.MetricsServer != "" {
		var err error
		observerFactory, err = observers.NewFactory(r.Spec.MetricsServer)
		if err != nil {
			c.recordEventErrorf(r, "Error building Prometheus client for %s %v", r.Spec.MetricsServer, err)
			return false
		}
	}
	observer := observerFactory.Observer(metricsProvider)

	// run metrics checks
	for _, metric := range r.Spec.CanaryAnalysis.Metrics {
		if metric.Interval == "" {
			metric.Interval = r.GetMetricInterval()
		}

		if metric.Name == "request-success-rate" {
			val, err := observer.GetRequestSuccessRate(toMetricModel(r, metric.Interval))
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for %s metric %s probably %s.%s is not receiving traffic",
						metricsProvider, metric.Name, r.Spec.TargetRef.Name, r.Namespace)
				} else {
					c.recordEventErrorf(r, "Prometheus query failed: %v", err)
				}
				return false
			}
			if float64(metric.Threshold) > val {
				c.recordEventWarningf(r, "Halt %s.%s advancement success rate %.2f%% < %v%%",
					r.Name, r.Namespace, val, metric.Threshold)
				return false
			}

			//c.recordEventInfof(r, "Check %s passed %.2f%% > %v%%", metric.Name, val, metric.Threshold)
		}

		if metric.Name == "request-duration" {
			val, err := observer.GetRequestDuration(toMetricModel(r, metric.Interval))
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for %s metric %s probably %s.%s is not receiving traffic",
						metricsProvider, metric.Name, r.Spec.TargetRef.Name, r.Namespace)
				} else {
					c.recordEventErrorf(r, "Prometheus query failed: %v", err)
				}
				return false
			}
			t := time.Duration(metric.Threshold) * time.Millisecond
			if val > t {
				c.recordEventWarningf(r, "Halt %s.%s advancement request duration %v > %v",
					r.Name, r.Namespace, val, t)
				return false
			}

			//c.recordEventInfof(r, "Check %s passed %v < %v", metric.Name, val, metric.Threshold)
		}

		// custom checks
		if metric.Query != "" {
			val, err := observerFactory.Client.RunQuery(metric.Query)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for custom metric: %s",
						metric.Name)
				} else {
					c.recordEventErrorf(r, "Prometheus query failed for %s: %v", metric.Name, err)
				}
				return false
			}
			if val > float64(metric.Threshold) {
				c.recordEventWarningf(r, "Halt %s.%s advancement %s %.2f > %v",
					r.Name, r.Namespace, metric.Name, val, metric.Threshold)
				return false
			}
		}
	}

	return true
}

func (c *Controller) runMetricChecks(r *flaggerv1.Canary) bool {
	for _, metric := range r.Spec.CanaryAnalysis.Metrics {
		if metric.TemplateRef != nil {
			namespace := r.Namespace
			if metric.TemplateRef.Namespace != "" {
				namespace = metric.TemplateRef.Namespace
			}

			template, err := c.flaggerClient.FlaggerV1beta1().MetricTemplates(namespace).Get(metric.TemplateRef.Name, metav1.GetOptions{})
			if err != nil {
				c.recordEventErrorf(r, "Metric template %s.%s error: %v", metric.TemplateRef.Name, namespace, err)
				return false
			}

			var credentials map[string][]byte
			if template.Spec.Provider.SecretRef != nil {
				secret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(template.Spec.Provider.SecretRef.Name, metav1.GetOptions{})
				if err != nil {
					c.recordEventErrorf(r, "Metric template %s.%s secret %s error: %v",
						metric.TemplateRef.Name, namespace, template.Spec.Provider.SecretRef.Name, err)
					return false
				}
				credentials = secret.Data
			}

			factory := providers.Factory{}
			provider, err := factory.Provider(template.Spec.Provider, credentials)
			if err != nil {
				c.recordEventErrorf(r, "Metric template %s.%s provider %s error: %v",
					metric.TemplateRef.Name, namespace, template.Spec.Provider.Type, err)
				return false
			}

			query, err := observers.RenderQuery(template.Spec.Query, toMetricModel(r, metric.Interval))
			if err != nil {
				c.recordEventErrorf(r, "Metric template %s.%s query render error: %v",
					metric.TemplateRef.Name, namespace, err)
				return false
			}

			val, err := provider.RunQuery(query)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for custom metric: %s",
						metric.Name)
				} else {
					c.recordEventErrorf(r, "Metric query failed for %s: %v", metric.Name, err)
				}
				return false
			}

			if val > metric.Threshold {
				c.recordEventWarningf(r, "Halt %s.%s advancement %s %.2f > %v",
					r.Name, r.Namespace, metric.Name, val, metric.Threshold)
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
	if canary.Status.FailedChecks >= canary.Spec.CanaryAnalysis.Threshold {
		c.recordEventWarningf(canary, "Rolling back %s.%s failed checks threshold reached %v",
			canary.Name, canary.Namespace, canary.Status.FailedChecks)
		c.sendNotification(canary, fmt.Sprintf("Failed checks threshold reached %v", canary.Status.FailedChecks),
			false, true)
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
