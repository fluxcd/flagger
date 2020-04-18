package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
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
		cn := value.(*flaggerv1.Canary)

		// format: <name>.<namespace>
		name := key.(string)
		current[name] = fmt.Sprintf("%s.%s", cn.Spec.TargetRef.Name, cn.Namespace)

		job, exists := c.jobs[name]
		// schedule new job for existing job with different analysis interval or non-existing job
		if (exists && job.GetCanaryAnalysisInterval() != cn.GetAnalysisInterval()) || !exists {
			if exists {
				job.Stop()
			}

			newJob := CanaryJob{
				Name:             cn.Name,
				Namespace:        cn.Namespace,
				function:         c.advanceCanary,
				done:             make(chan bool),
				ticker:           time.NewTicker(cn.GetAnalysisInterval()),
				analysisInterval: cn.GetAnalysisInterval(),
			}

			c.jobs[name] = newJob
			newJob.Start()
		}

		// compute canaries per namespace total
		t, ok := stats[cn.Namespace]
		if !ok {
			stats[cn.Namespace] = 1
		} else {
			stats[cn.Namespace] = t + 1
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
				c.logger.With("canary", canaryName).
					Errorf("Bad things will happen! Found more than one canary with the same target %s", targetName)
			}
		}
	}

	// set total canaries per namespace metric
	for k, v := range stats {
		c.recorder.SetTotal(k, v)
	}
}

func (c *Controller) advanceCanary(name string, namespace string) {
	begin := time.Now()
	// check if the canary exists
	cd, err := c.flaggerClient.FlaggerV1beta1().Canaries(namespace).Get(context.TODO(), name, metav1.GetOptions{})
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
	kubeRouter := c.routerFactory.KubernetesRouter(cd.Spec.TargetRef.Kind, labelSelector, ports)
	if err := kubeRouter.Initialize(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// create primary
	err = canaryController.Initialize(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// init mesh router
	meshRouter := c.routerFactory.MeshRouter(provider)

	// create or update svc
	if err := kubeRouter.Reconcile(cd); err != nil {
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
	if !cd.SkipAnalysis() {
		if err := canaryController.IsPrimaryReady(cd); err != nil {
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
		}
		return
	}

	// check canary status
	var retriable = true
	retriable, err = canaryController.IsCanaryReady(cd)
	if err != nil && retriable {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// check if analysis should be skipped
	if skip := c.shouldSkipAnalysis(cd, canaryController, meshRouter); skip {
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
		if err := canaryController.ScaleToZero(cd); err != nil {
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
		!(cd.GetAnalysis().Mirror && mirrored) {
		c.recordEventInfof(cd, "Starting canary analysis for %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)

		// run pre-rollout web hooks
		if ok := c.runPreRolloutHooks(cd); !ok {
			if err := canaryController.SetStatusFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
			}
			return
		}
	} else {
		if ok := c.runAnalysis(cd); !ok {
			if err := canaryController.SetStatusFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
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
		c.runAB(cd, canaryController, meshRouter)
		return
	}

	// strategy: Blue/Green
	if cd.GetAnalysis().Iterations > 0 {
		c.runBlueGreen(cd, canaryController, meshRouter, provider, mirrored)
		return
	}

	// strategy: Canary progressive traffic increase
	if cd.GetAnalysis().StepWeight > 0 {
		c.runCanary(cd, canaryController, meshRouter, mirrored, canaryWeight, primaryWeight, maxWeight)
	}

}

func (c *Controller) runCanary(canary *flaggerv1.Canary, canaryController canary.Controller,
	meshRouter router.Interface, mirrored bool, canaryWeight int, primaryWeight int, maxWeight int) {
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	// increase traffic weight
	if canaryWeight < maxWeight {
		// If in "mirror" mode, do one step of mirroring before shifting traffic to canary.
		// When mirroring, all requests go to primary and canary, but only responses from
		// primary go back to the user.
		if canary.GetAnalysis().Mirror && canaryWeight == 0 {
			if !mirrored {
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

func (c *Controller) runAB(canary *flaggerv1.Canary, canaryController canary.Controller,
	meshRouter router.Interface) {
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

func (c *Controller) runBlueGreen(canary *flaggerv1.Canary, canaryController canary.Controller,
	meshRouter router.Interface, provider string, mirrored bool) {
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	// increment iterations
	if canary.GetAnalysis().Iterations > canary.Status.Iterations {
		// If in "mirror" mode, mirror requests during the entire B/G canary test
		if provider != "kubernetes" &&
			canary.GetAnalysis().Mirror && !mirrored {
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

func (c *Controller) shouldSkipAnalysis(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface) bool {
	if !canary.SkipAnalysis() {
		return false
	}

	// route all traffic to primary
	primaryWeight := 100
	canaryWeight := 0
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
	if err := canaryController.ScaleToZero(canary); err != nil {
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

	var err error
	canary, err = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(context.TODO(), canary.Name, metav1.GetOptions{})
	if err != nil {
		c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
		return false
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
	if err := canaryController.ScaleToZero(canary); err != nil {
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
