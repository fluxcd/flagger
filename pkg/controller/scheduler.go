package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/weaveworks/flagger/pkg/router"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	cd, err := c.flaggerClient.FlaggerV1alpha3().Canaries(namespace).Get(name, v1.GetOptions{})
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

	// create primary deployment and hpa if needed
	// skip primary check for Istio since the deployment will become ready after the ClusterIP are created
	skipPrimaryCheck := false
	if skipLivenessChecks || strings.Contains(provider, "istio") {
		skipPrimaryCheck = true
	}
	label, ports, err := c.deployer.Initialize(cd, skipPrimaryCheck)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// init routers
	meshRouter := c.routerFactory.MeshRouter(provider)

	// create or update ClusterIP services
	if err := c.routerFactory.KubernetesRouter(label, ports).Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// create or update virtual service
	if err := meshRouter.Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	shouldAdvance, err := c.shouldAdvance(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	if !shouldAdvance {
		c.recorder.SetStatus(cd, cd.Status.Phase)
		return
	}

	// check gates
	if isApproved := c.runConfirmRolloutHooks(cd); !isApproved {
		return
	}

	// set max weight default value to 100%
	maxWeight := 100
	if cd.Spec.CanaryAnalysis.MaxWeight > 0 {
		maxWeight = cd.Spec.CanaryAnalysis.MaxWeight
	}

	// check primary deployment status
	if !skipLivenessChecks {
		if _, err := c.deployer.IsPrimaryReady(cd); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
	}

	// check if virtual service exists
	// and if it contains weighted destination routes to the primary and canary services
	primaryWeight, canaryWeight, err := meshRouter.GetRoutes(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	c.recorder.SetWeight(cd, primaryWeight, canaryWeight)

	// check if canary analysis should start (canary revision has changes) or continue
	if ok := c.checkCanaryStatus(cd, shouldAdvance); !ok {
		return
	}

	// check if canary revision changed during analysis
	if restart := c.hasCanaryRevisionChanged(cd); restart {
		c.recordEventInfof(cd, "New revision detected! Restarting analysis for %s.%s",
			cd.Spec.TargetRef.Name, cd.Namespace)

		// route all traffic back to primary
		primaryWeight = 100
		canaryWeight = 0
		if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight); err != nil {
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
		if err := c.deployer.SyncStatus(cd, status); err != nil {
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
		retriable, err = c.deployer.IsCanaryReady(cd)
		if err != nil && retriable {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
	}

	// check if analysis should be skipped
	if skip := c.shouldSkipAnalysis(cd, meshRouter, primaryWeight, canaryWeight); skip {
		return
	}

	// scale canary to zero if analysis has succeeded
	if cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		if err := c.deployer.Scale(cd, 0); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// set status to succeeded
		if err := c.deployer.SetStatusPhase(cd, flaggerv1.CanaryPhaseSucceeded); err != nil {
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
		if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recorder.SetWeight(cd, primaryWeight, canaryWeight)
		c.recordEventWarningf(cd, "Canary failed! Scaling down %s.%s",
			cd.Name, cd.Namespace)

		// shutdown canary
		if err := c.deployer.Scale(cd, 0); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// mark canary as failed
		if err := c.deployer.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseFailed, CanaryWeight: 0}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Errorf("%v", err)
			return
		}

		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseFailed)
		c.runPostRolloutHooks(cd, flaggerv1.CanaryPhaseFailed)
		return
	}

	// check if the canary success rate is above the threshold
	// skip check if no traffic is routed to canary
	if canaryWeight == 0 {
		c.recordEventInfof(cd, "Starting canary analysis for %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)

		// run pre-rollout web hooks
		if ok := c.runPreRolloutHooks(cd); !ok {
			if err := c.deployer.SetStatusFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}
	} else {
		if ok := c.analyseCanary(cd); !ok {
			if err := c.deployer.SetStatusFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}
	}

	// canary fix routing: A/B testing
	if len(cd.Spec.CanaryAnalysis.Match) > 0 || cd.Spec.CanaryAnalysis.Iterations > 0 {
		// route traffic to canary and increment iterations
		if cd.Spec.CanaryAnalysis.Iterations > cd.Status.Iterations {
			if err := meshRouter.SetRoutes(cd, 0, 100); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recorder.SetWeight(cd, 0, 100)

			if err := c.deployer.SetStatusIterations(cd, cd.Status.Iterations+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recordEventInfof(cd, "Advance %s.%s canary iteration %v/%v",
				cd.Name, cd.Namespace, cd.Status.Iterations+1, cd.Spec.CanaryAnalysis.Iterations)
			return
		}

		// promote canary - max iterations reached
		if cd.Spec.CanaryAnalysis.Iterations == cd.Status.Iterations {
			c.recordEventInfof(cd, "Copying %s.%s template spec to %s.%s",
				cd.Spec.TargetRef.Name, cd.Namespace, primaryName, cd.Namespace)
			if err := c.deployer.Promote(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			// increment iterations
			if err := c.deployer.SetStatusIterations(cd, cd.Status.Iterations+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}

		// route all traffic to primary
		if cd.Spec.CanaryAnalysis.Iterations < cd.Status.Iterations {
			primaryWeight = 100
			canaryWeight = 0
			if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			c.recorder.SetWeight(cd, primaryWeight, canaryWeight)

			// update status phase
			if err := c.deployer.SetStatusPhase(cd, flaggerv1.CanaryPhaseFinalising); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}

			c.recordEventInfof(cd, "Routing all traffic to primary")
			return
		}

		return
	}

	// canary incremental traffic weight
	if canaryWeight < maxWeight {
		primaryWeight -= cd.Spec.CanaryAnalysis.StepWeight
		if primaryWeight < 0 {
			primaryWeight = 0
		}
		canaryWeight += cd.Spec.CanaryAnalysis.StepWeight
		if primaryWeight > 100 {
			primaryWeight = 100
		}

		if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// update weight status
		if err := c.deployer.SetStatusWeight(cd, canaryWeight); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recorder.SetWeight(cd, primaryWeight, canaryWeight)
		c.recordEventInfof(cd, "Advance %s.%s canary weight %v", cd.Name, cd.Namespace, canaryWeight)

		// promote canary
		if canaryWeight >= maxWeight {
			c.recordEventInfof(cd, "Copying %s.%s template spec to %s.%s",
				cd.Spec.TargetRef.Name, cd.Namespace, primaryName, cd.Namespace)
			if err := c.deployer.Promote(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
		}
	} else {
		// route all traffic to primary
		primaryWeight = 100
		canaryWeight = 0
		if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
		c.recorder.SetWeight(cd, primaryWeight, canaryWeight)

		// update status phase
		if err := c.deployer.SetStatusPhase(cd, flaggerv1.CanaryPhaseFinalising); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recordEventInfof(cd, "Routing all traffic to primary")
		return
	}
}

func (c *Controller) shouldSkipAnalysis(cd *flaggerv1.Canary, meshRouter router.Interface, primaryWeight int, canaryWeight int) bool {
	if !cd.Spec.SkipAnalysis {
		return false
	}

	// route all traffic to primary
	primaryWeight = 100
	canaryWeight = 0
	if err := meshRouter.SetRoutes(cd, primaryWeight, canaryWeight); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}
	c.recorder.SetWeight(cd, primaryWeight, canaryWeight)

	// copy spec and configs from canary to primary
	c.recordEventInfof(cd, "Copying %s.%s template spec to %s-primary.%s",
		cd.Spec.TargetRef.Name, cd.Namespace, cd.Spec.TargetRef.Name, cd.Namespace)
	if err := c.deployer.Promote(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}

	// shutdown canary
	if err := c.deployer.Scale(cd, 0); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return false
	}

	// update status phase
	if err := c.deployer.SetStatusPhase(cd, flaggerv1.CanaryPhaseSucceeded); err != nil {
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

func (c *Controller) shouldAdvance(cd *flaggerv1.Canary) (bool, error) {
	if cd.Status.LastAppliedSpec == "" ||
		cd.Status.Phase == flaggerv1.CanaryPhaseInitializing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseWaiting ||
		cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true, nil
	}

	newDep, err := c.deployer.HasDeploymentChanged(cd)
	if err != nil {
		return false, err
	}
	if newDep {
		return newDep, nil
	}

	newCfg, err := c.deployer.ConfigTracker.HasConfigChanged(cd)
	if err != nil {
		return false, err
	}

	return newCfg, nil

}

func (c *Controller) checkCanaryStatus(cd *flaggerv1.Canary, shouldAdvance bool) bool {
	c.recorder.SetStatus(cd, cd.Status.Phase)
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if err := c.deployer.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseInitialized}); err != nil {
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
		if err := c.deployer.Scale(cd, 1); err != nil {
			c.recordEventErrorf(cd, "%v", err)
			return false
		}
		if err := c.deployer.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing}); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseProgressing)
		return false
	}
	return false
}

func (c *Controller) hasCanaryRevisionChanged(cd *flaggerv1.Canary) bool {
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing {
		if diff, _ := c.deployer.HasDeploymentChanged(cd); diff {
			return true
		}
		if diff, _ := c.deployer.ConfigTracker.HasConfigChanged(cd); diff {
			return true
		}
	}
	return false
}

func (c *Controller) runConfirmRolloutHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.Spec.CanaryAnalysis.Webhooks {
		if webhook.Type == flaggerv1.ConfirmRolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				if canary.Status.Phase != flaggerv1.CanaryPhaseWaiting {
					if err := c.deployer.SetStatusPhase(canary, flaggerv1.CanaryPhaseWaiting); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
					}
					c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for approval %s",
						canary.Name, canary.Namespace, webhook.Name)
					c.sendNotification(canary, "Canary is waiting for approval.", false, false)
				}
				return false
			} else {
				if canary.Status.Phase == flaggerv1.CanaryPhaseWaiting {
					if err := c.deployer.SetStatusPhase(canary, flaggerv1.CanaryPhaseProgressing); err != nil {
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
	observer := c.observerFactory.Observer(metricsProvider)

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
					c.recordEventErrorf(r, "Metrics server %s query failed: %v", c.observerFactory.Client.GetMetricsServer(), err)
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
					c.recordEventErrorf(r, "Metrics server %s query failed: %v", c.observerFactory.Client.GetMetricsServer(), err)
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
			val, err := c.observerFactory.Client.RunQuery(metric.Query)
			if err != nil {
				if strings.Contains(err.Error(), "no values found") {
					c.recordEventWarningf(r, "Halt advancement no values found for metric %s probably %s.%s is not receiving traffic",
						metric.Name, r.Spec.TargetRef.Name, r.Namespace)
				} else {
					c.recordEventErrorf(r, "Metrics server %s query failed for %s: %v", c.observerFactory.Client.GetMetricsServer(), metric.Name, err)
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
