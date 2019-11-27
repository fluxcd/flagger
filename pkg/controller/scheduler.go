package controller

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	"github.com/weaveworks/flagger/pkg/canary"
	"github.com/weaveworks/flagger/pkg/metrics"
	"github.com/weaveworks/flagger/pkg/router"
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
	cd, err := c.flaggerClient.FlaggerV1alpha3().Canaries(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		c.logger.With("canary", fmt.Sprintf("%s.%s", name, namespace)).
			Errorf("Canary %s.%s not found", name, namespace)
		return
	}

	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	// override the global provider if one is specified in the canary spec
	provider := c.meshProvider
	if cd.Spec.Provider != "" {
		provider = cd.Spec.Provider
	}

	// init controller based on target kind
	canaryController := c.canaryFactory.Controller(cd.Spec.TargetRef.Kind)

	// create primary deployment and hpa if needed
	// skip primary check for Istio since the deployment will become ready after the ClusterIP are created
	skipPrimaryCheck := false
	if skipLivenessChecks || strings.Contains(provider, "istio") || strings.Contains(provider, "appmesh") {
		skipPrimaryCheck = true
	}
	labelSelector, ports, err := canaryController.Initialize(cd, skipPrimaryCheck)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// init routers
	meshRouter := c.routerFactory.MeshRouter(provider)

	// create or update ClusterIP services
	if err := c.routerFactory.KubernetesRouter(cd.Spec.TargetRef.Kind, labelSelector, map[string]string{}, ports).Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// create or update virtual service
	if err := meshRouter.Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// check for deployment spec or configs changes
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

	defer func() {
		c.recorder.SetDuration(cd, time.Since(begin))
	}()

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

		if cd.Status.FailedChecks >= cd.Spec.CanaryAnalysis.Threshold {
			c.recordEventWarningf(cd, "Rolling back %s.%s failed checks threshold reached %v",
				cd.Name, cd.Namespace, cd.Status.FailedChecks)
			c.sendNotification(cd, fmt.Sprintf("Failed checks threshold reached %v", cd.Status.FailedChecks),
				false, true)
		}

		if !retriable {
			c.recordEventWarningf(cd, "Rolling back %s.%s progress deadline exceeded %v",
				cd.Name, cd.Namespace, err)
			c.sendNotification(cd, fmt.Sprintf("Progress deadline exceeded %v", err),
				false, true)
		}

		// route all traffic back to primary
		primaryWeight = 100
		canaryWeight = 0
		if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight, false); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recorder.SetWeight(cd, primaryWeight, canaryWeight)
		c.recordEventWarningf(cd, "Canary failed! Scaling down %s.%s",
			cd.Name, cd.Namespace)

		// shutdown canary
		if err := canaryController.Scale(cd, 0); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// mark canary as failed
		if err := canaryController.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseFailed, CanaryWeight: 0}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Errorf("%v", err)
			return
		}

		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseFailed)
		c.runPostRolloutHooks(cd, flaggerv1.CanaryPhaseFailed)
		return
	}

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
		if ok := c.analyseCanary(cd); !ok {
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
		// route traffic to canary and increment iterations
		if cd.Spec.CanaryAnalysis.Iterations > cd.Status.Iterations {
			if err := meshRouter.SetRoutes(cd, 0, 100, false); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recorder.SetWeight(cd, 0, 100)

			if err := canaryController.SetStatusIterations(cd, cd.Status.Iterations+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recordEventInfof(cd, "Advance %s.%s canary iteration %v/%v",
				cd.Name, cd.Namespace, cd.Status.Iterations+1, cd.Spec.CanaryAnalysis.Iterations)
			return
		}

		// check promotion gate
		if promote := c.runConfirmPromotionHooks(cd); !promote {
			return
		}

		// promote canary - max iterations reached
		if cd.Spec.CanaryAnalysis.Iterations == cd.Status.Iterations {
			c.recordEventInfof(cd, "Copying %s.%s template spec to %s.%s",
				cd.Spec.TargetRef.Name, cd.Namespace, primaryName, cd.Namespace)
			if err := canaryController.Promote(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			// update status phase
			if err := canaryController.SetStatusPhase(cd, flaggerv1.CanaryPhasePromoting); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}

		return
	}

	// strategy: Blue/Green
	if cd.Spec.CanaryAnalysis.Iterations > 0 {
		// increment iterations
		if cd.Spec.CanaryAnalysis.Iterations > cd.Status.Iterations {
			// If in "mirror" mode, mirror requests during the entire B/G canary test
			if provider != "kubernetes" &&
				cd.Spec.CanaryAnalysis.Mirror == true && mirrored == false {
				if err := meshRouter.SetRoutes(cd, 100, 0, true); err != nil {
					c.recordEventWarningf(cd, "%v", err)
				}
				c.logger.With("canary", fmt.Sprintf("%s.%s", name, namespace)).
					Infof("Start traffic mirroring")
			}
			if err := canaryController.SetStatusIterations(cd, cd.Status.Iterations+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recordEventInfof(cd, "Advance %s.%s canary iteration %v/%v",
				cd.Name, cd.Namespace, cd.Status.Iterations+1, cd.Spec.CanaryAnalysis.Iterations)
			return
		}

		// check promotion gate
		if promote := c.runConfirmPromotionHooks(cd); !promote {
			return
		}

		// route all traffic to canary - max iterations reached
		if cd.Spec.CanaryAnalysis.Iterations == cd.Status.Iterations {
			if provider != "kubernetes" {
				if cd.Spec.CanaryAnalysis.Mirror {
					c.recordEventInfof(cd, "Stop traffic mirroring and route all traffic to canary")
				} else {
					c.recordEventInfof(cd, "Routing all traffic to canary")
				}
				if err := meshRouter.SetRoutes(cd, 0, 100, false); err != nil {
					c.recordEventWarningf(cd, "%v", err)
					return
				}
				c.recorder.SetWeight(cd, 0, 100)
			}

			// increment iterations
			if err := canaryController.SetStatusIterations(cd, cd.Status.Iterations+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}

		// promote canary - max iterations reached
		if cd.Spec.CanaryAnalysis.Iterations < cd.Status.Iterations {
			c.recordEventInfof(cd, "Copying %s.%s template spec to %s.%s",
				cd.Spec.TargetRef.Name, cd.Namespace, primaryName, cd.Namespace)
			if err := canaryController.Promote(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			// update status phase
			if err := canaryController.SetStatusPhase(cd, flaggerv1.CanaryPhasePromoting); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}

		return
	}

	// strategy: Canary progressive traffic increase
	if cd.Spec.CanaryAnalysis.StepWeight > 0 {
		// increase traffic weight
		if canaryWeight < maxWeight {
			// If in "mirror" mode, do one step of mirroring before shifting traffic to canary.
			// When mirroring, all requests go to primary and canary, but only responses from
			// primary go back to the user.
			if cd.Spec.CanaryAnalysis.Mirror && canaryWeight == 0 {
				if mirrored == false {
					mirrored = true
					primaryWeight = 100
					canaryWeight = 0
				} else {
					mirrored = false
					primaryWeight = 100 - cd.Spec.CanaryAnalysis.StepWeight
					canaryWeight = cd.Spec.CanaryAnalysis.StepWeight
				}
				c.logger.With("canary", fmt.Sprintf("%s.%s", name, namespace)).
					Infof("Running mirror step %d/%d/%t", primaryWeight, canaryWeight, mirrored)
			} else {

				primaryWeight -= cd.Spec.CanaryAnalysis.StepWeight
				if primaryWeight < 0 {
					primaryWeight = 0
				}
				canaryWeight += cd.Spec.CanaryAnalysis.StepWeight
				if canaryWeight > 100 {
					canaryWeight = 100
				}
			}

			if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight, mirrored); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			if err := canaryController.SetStatusWeight(cd, canaryWeight); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			c.recorder.SetWeight(cd, primaryWeight, canaryWeight)
			c.recordEventInfof(cd, "Advance %s.%s canary weight %v", cd.Name, cd.Namespace, canaryWeight)
			return
		}

		// promote canary - max weight reached
		if canaryWeight >= maxWeight {
			// check promotion gate
			if promote := c.runConfirmPromotionHooks(cd); !promote {
				return
			}

			// update primary spec
			c.recordEventInfof(cd, "Copying %s.%s template spec to %s.%s",
				cd.Spec.TargetRef.Name, cd.Namespace, primaryName, cd.Namespace)
			if err := canaryController.Promote(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			// update status phase
			if err := canaryController.SetStatusPhase(cd, flaggerv1.CanaryPhasePromoting); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			return
		}

	}

}

func (c *Controller) shouldSkipAnalysis(cd *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface, primaryWeight int, canaryWeight int) bool {
	if !cd.Spec.SkipAnalysis {
		return false
	}

	// route all traffic to primary
	primaryWeight = 100
	canaryWeight = 0
	if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight, false); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}
	c.recorder.SetWeight(cd, primaryWeight, canaryWeight)

	// copy spec and configs from canary to primary
	c.recordEventInfof(cd, "Copying %s.%s template spec to %s-primary.%s",
		cd.Spec.TargetRef.Name, cd.Namespace, cd.Spec.TargetRef.Name, cd.Namespace)
	if err := canaryController.Promote(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}

	// shutdown canary
	if err := canaryController.Scale(cd, 0); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}

	// update status phase
	if err := canaryController.SetStatusPhase(cd, flaggerv1.CanaryPhaseSucceeded); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}

	// notify
	c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseSucceeded)
	c.recordEventInfof(cd, "Promotion completed! Canary analysis was skipped for %s.%s",
		cd.Spec.TargetRef.Name, cd.Namespace)
	c.sendNotification(cd, "Canary analysis was skipped, promotion finished.",
		false, false)

	return true
}

func (c *Controller) shouldAdvance(cd *flaggerv1.Canary, canaryController canary.Controller) (bool, error) {
	if cd.Status.LastAppliedSpec == "" ||
		cd.Status.Phase == flaggerv1.CanaryPhaseInitializing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseWaiting ||
		cd.Status.Phase == flaggerv1.CanaryPhasePromoting ||
		cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true, nil
	}

	newTarget, err := canaryController.HasTargetChanged(cd)
	if err != nil {
		return false, err
	}
	if newTarget {
		return newTarget, nil
	}

	newCfg, err := canaryController.HaveDependenciesChanged(cd)
	if err != nil {
		return false, err
	}

	return newCfg, nil

}

func (c *Controller) checkCanaryStatus(cd *flaggerv1.Canary, canaryController canary.Controller, shouldAdvance bool) bool {
	c.recorder.SetStatus(cd, cd.Status.Phase)
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		cd.Status.Phase == flaggerv1.CanaryPhasePromoting ||
		cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if err := canaryController.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseInitialized}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseInitialized)
		c.recordEventInfof(cd, "Initialization done! %s.%s", cd.Name, cd.Namespace)
		c.sendNotification(cd, "New deployment detected, initialization completed.",
			true, false)
		return false
	}

	if shouldAdvance {
		c.recordEventInfof(cd, "New revision detected! Scaling up %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		c.sendNotification(cd, "New revision detected, starting canary analysis.",
			true, false)
		if err := canaryController.ScaleFromZero(cd); err != nil {
			c.recordEventErrorf(cd, "%v", err)
			return false
		}
		if err := canaryController.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseProgressing)
		return false
	}
	return false
}

func (c *Controller) hasCanaryRevisionChanged(cd *flaggerv1.Canary, canaryController canary.Controller) bool {
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing {
		if diff, _ := canaryController.HasTargetChanged(cd); diff {
			return true
		}
		if diff, _ := canaryController.HaveDependenciesChanged(cd); diff {
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

func (c *Controller) analyseCanary(r *flaggerv1.Canary) bool {
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

	// override the global provider if one is specified in the canary spec
	metricsProvider := c.meshProvider
	if r.Spec.Provider != "" {
		metricsProvider = r.Spec.Provider

		// set the metrics server to Linkerd Prometheus when Linkerd is the default mesh provider
		if strings.Contains(c.meshProvider, "linkerd") {
			metricsProvider = "linkerd"
		}
	}

	// create observer based on the mesh provider
	observerFactory := c.observerFactory

	// override the global metrics server if one is specified in the canary spec
	metricsServer := c.observerFactory.Client.GetMetricsServer()
	if r.Spec.MetricsServer != "" {
		metricsServer = r.Spec.MetricsServer
		var err error
		observerFactory, err = metrics.NewFactory(metricsServer, metricsProvider, 5*time.Second)
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
			val, err := observer.GetRequestSuccessRate(r.Spec.TargetRef.Name, r.Namespace, metric.Interval)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for metric %s probably %s.%s is not receiving traffic",
						metric.Name, r.Spec.TargetRef.Name, r.Namespace)
				} else {
					c.recordEventErrorf(r, "Metrics server %s query failed: %v", metricsServer, err)
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
			val, err := observer.GetRequestDuration(r.Spec.TargetRef.Name, r.Namespace, metric.Interval)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for metric %s probably %s.%s is not receiving traffic",
						metric.Name, r.Spec.TargetRef.Name, r.Namespace)
				} else {
					c.recordEventErrorf(r, "Metrics server %s query failed: %v", metricsServer, err)
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
					c.recordEventErrorf(r, "Metrics server %s query failed for %s: %v", metricsServer, metric.Name, err)
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
