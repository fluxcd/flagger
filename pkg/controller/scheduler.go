/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/canary"
	"github.com/fluxcd/flagger/pkg/router"
)

func (c *Controller) min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Controller) maxWeight(canary *flaggerv1.Canary) int {
	var stepWeightsLen = len(canary.GetAnalysis().StepWeights)
	if stepWeightsLen > 0 {
		return c.min(c.totalWeight(canary), canary.GetAnalysis().StepWeights[stepWeightsLen-1])
	}
	if canary.GetAnalysis().MaxWeight > 0 {
		return canary.GetAnalysis().MaxWeight
	}
	// set max weight default value to total weight
	return c.totalWeight(canary)
}

func (c *Controller) totalWeight(canary *flaggerv1.Canary) int {
	// set total weight default value to 100%
	return 100
}

func (c *Controller) nextStepWeight(canary *flaggerv1.Canary, canaryWeight int) int {
	var stepWeightsLen = len(canary.GetAnalysis().StepWeights)
	if canary.GetAnalysis().StepWeight > 0 || stepWeightsLen == 0 {
		return canary.GetAnalysis().StepWeight
	}

	totalWeight := c.totalWeight(canary)
	maxStep := totalWeight - canaryWeight

	// If maxStep is zero we need to promote, so any non zero step weight will move the canary to promotion.
	// This is the same use case as the last step via StepWeight.
	if maxStep == 0 {
		return 1
	}

	// return min of maxStep and the calculated step to avoid going above totalWeight

	// initial step
	if canaryWeight == 0 {
		return c.min(maxStep, canary.GetAnalysis().StepWeights[0])
	}

	// find the current step and return the difference in weight
	for i := 0; i < stepWeightsLen-1; i++ {
		if canary.GetAnalysis().StepWeights[i] == canaryWeight {
			return c.min(maxStep, canary.GetAnalysis().StepWeights[i+1]-canaryWeight)
		}
	}

	return maxStep
}

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

	if cd.Spec.Suspend {
		msg := "skipping canary run as object is suspended"
		c.logger.With("canary", fmt.Sprintf("%s.%s", name, namespace)).
			Debug(msg)
		c.recordEventInfof(cd, msg)
		return
	}

	// override the global provider if one is specified in the canary spec
	provider := c.meshProvider
	if cd.Spec.Provider != "" {
		provider = cd.Spec.Provider
	}

	// init controller based on target kind
	canaryController := c.canaryFactory.Controller(cd.Spec.TargetRef.Kind)

	labelSelector, labelValue, ports, err := canaryController.GetMetadata(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	var scalerReconciler canary.ScalerReconciler
	if cd.Spec.AutoscalerRef != nil {
		scalerReconciler = c.canaryFactory.ScalerReconciler(cd.Spec.AutoscalerRef.Kind)
	}

	// init Kubernetes router
	kubeRouter := c.routerFactory.KubernetesRouter(cd.Spec.TargetRef.Kind, labelSelector, labelValue, ports)

	// reconcile the canary/primary services
	if err := kubeRouter.Initialize(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// check metric servers' availability
	if !cd.SkipAnalysis() && (cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing) {
		if err := c.checkMetricProviderAvailability(cd); err != nil {
			c.recordEventErrorf(cd, "Error checking metric providers: %v", err)
		}
	}

	// init mesh router
	meshRouter := c.routerFactory.MeshRouter(provider, labelSelector)

	// register the AppMesh VirtualNodes before creating the primary deployment
	// otherwise the pods will not be injected with the Envoy proxy
	if strings.HasPrefix(provider, flaggerv1.AppMeshProvider) {
		if err := meshRouter.Reconcile(cd); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
	}

	// create primary workload
	err = canaryController.Initialize(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	if scalerReconciler != nil {
		err = scalerReconciler.ReconcilePrimaryScaler(cd, true)
		if err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
		if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
			err = scalerReconciler.PauseTargetScaler(cd)
			if err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
		}
	}

	// change the apex service pod selector to primary
	if err := kubeRouter.Reconcile(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// take over an existing virtual service or ingress
	// runs after the primary is ready to ensure zero downtime
	if !strings.HasPrefix(provider, flaggerv1.AppMeshProvider) {
		if err := meshRouter.Reconcile(cd); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
	}

	// set canary phase to initialized and sync the status
	if err = c.setPhaseInitialized(cd, canaryController); err != nil {
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

	maxWeight := c.maxWeight(cd)

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
	if ok := c.checkCanaryStatus(cd, canaryController, scalerReconciler, shouldAdvance); !ok {
		return
	}

	// check if canary revision changed during analysis
	if restart := c.hasCanaryRevisionChanged(cd, canaryController); restart {
		c.recordEventInfof(cd, "New revision detected! Restarting analysis for %s.%s",
			cd.Spec.TargetRef.Name, cd.Namespace)

		// route all traffic back to primary
		primaryWeight = c.totalWeight(cd)
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
	if skip := c.shouldSkipAnalysis(cd, canaryController, meshRouter, scalerReconciler, err, retriable); skip {
		return
	}

	// check if we should rollback
	if cd.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		cd.Status.Phase == flaggerv1.CanaryPhaseWaiting ||
		cd.Status.Phase == flaggerv1.CanaryPhaseWaitingPromotion {
		if ok := c.runRollbackHooks(cd, cd.Status.Phase); ok {
			c.recordEventWarningf(cd, "Rolling back %s.%s manual webhook invoked", cd.Name, cd.Namespace)
			c.alert(cd, "Rolling back manual webhook invoked", false, flaggerv1.SeverityWarn)
			c.rollback(cd, canaryController, meshRouter, scalerReconciler)
			return
		}
	}

	// route traffic back to primary if analysis has succeeded
	if cd.Status.Phase == flaggerv1.CanaryPhasePromoting {
		if scalerReconciler != nil {
			if err := scalerReconciler.ReconcilePrimaryScaler(cd, false); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
		}
		c.runPromotionTrafficShift(cd, canaryController, meshRouter, provider, canaryWeight, primaryWeight)
		return
	}

	// scale canary to zero if promotion has finished
	if cd.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		if scalerReconciler != nil {
			if err := scalerReconciler.PauseTargetScaler(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
		}
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
	if (cd.Status.Phase == flaggerv1.CanaryPhaseProgressing || cd.Status.Phase == flaggerv1.CanaryPhaseWaitingPromotion) &&
		(!retriable || cd.Status.FailedChecks >= cd.GetAnalysisThreshold()) {
		if !retriable {
			c.recordEventWarningf(cd, "Rolling back %s.%s progress deadline exceeded %v",
				cd.Name, cd.Namespace, err)
			c.alert(cd, fmt.Sprintf("Progress deadline exceeded %v", err),
				false, flaggerv1.SeverityError)
		}
		c.rollback(cd, canaryController, meshRouter, scalerReconciler)
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
	if provider == flaggerv1.KubernetesProvider {
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
	if c.nextStepWeight(cd, canaryWeight) > 0 {
		// run hook only if traffic is not mirrored
		if !mirrored &&
			(cd.Status.Phase != flaggerv1.CanaryPhasePromoting &&
				cd.Status.Phase != flaggerv1.CanaryPhaseWaitingPromotion &&
				cd.Status.Phase != flaggerv1.CanaryPhaseFinalising) {
			if promote := c.runConfirmTrafficIncreaseHooks(cd); !promote {
				return
			}
		}
		c.runCanary(cd, canaryController, meshRouter, mirrored, canaryWeight, primaryWeight, maxWeight)
	}

}

func (c *Controller) runPromotionTrafficShift(canary *flaggerv1.Canary, canaryController canary.Controller,
	meshRouter router.Interface, provider string, canaryWeight int, primaryWeight int) {
	// finalize promotion since no traffic shifting is possible for Kubernetes CNI
	if provider == flaggerv1.KubernetesProvider {
		if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseFinalising); err != nil {
			c.recordEventWarningf(canary, "%v", err)
		}
		return
	}

	// route all traffic to primary in one go when promotion step wight is not set
	if canary.Spec.Analysis.StepWeightPromotion == 0 {
		c.recordEventInfof(canary, "Routing all traffic to primary")
		if err := meshRouter.SetRoutes(canary, c.totalWeight(canary), 0, false); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recorder.SetWeight(canary, c.totalWeight(canary), 0)
		if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseFinalising); err != nil {
			c.recordEventWarningf(canary, "%v", err)
		}
		return
	}

	// increment the primary traffic weight until it reaches total weight
	if canaryWeight > 0 {
		primaryWeight += canary.GetAnalysis().StepWeightPromotion
		if primaryWeight > c.totalWeight(canary) {
			primaryWeight = c.totalWeight(canary)
		}
		canaryWeight -= canary.GetAnalysis().StepWeightPromotion
		if canaryWeight < 0 {
			canaryWeight = 0
		}
		if err := meshRouter.SetRoutes(canary, primaryWeight, canaryWeight, false); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recorder.SetWeight(canary, primaryWeight, canaryWeight)
		c.recordEventInfof(canary, "Advance %s.%s primary weight %v", canary.Name, canary.Namespace, primaryWeight)

		// finalize promotion
		if primaryWeight == c.totalWeight(canary) {
			if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseFinalising); err != nil {
				c.recordEventWarningf(canary, "%v", err)
			}
		} else {
			if err := canaryController.SetStatusWeight(canary, canaryWeight); err != nil {
				c.recordEventWarningf(canary, "%v", err)
			}
		}
	}

	return

}

func (c *Controller) runCanary(canary *flaggerv1.Canary, canaryController canary.Controller,
	meshRouter router.Interface, mirrored bool, canaryWeight int, primaryWeight int, maxWeight int) {
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	// increase traffic weight
	if canaryWeight < maxWeight {
		// If in "mirror" mode, do one step of mirroring before shifting traffic to canary.
		// When mirroring, all requests go to primary and canary, but only responses from
		// primary go back to the user.

		var nextStepWeight int
		nextStepWeight = c.nextStepWeight(canary, canaryWeight)
		if canary.GetAnalysis().Mirror && canaryWeight == 0 {
			if !mirrored {
				mirrored = true
				primaryWeight = c.totalWeight(canary)
				canaryWeight = 0
			} else {
				mirrored = false
				primaryWeight = c.totalWeight(canary) - nextStepWeight
				canaryWeight = nextStepWeight
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Running mirror step %d/%d/%t", primaryWeight, canaryWeight, mirrored)
		} else {

			primaryWeight -= nextStepWeight
			if primaryWeight < 0 {
				primaryWeight = 0
			}
			canaryWeight += nextStepWeight
			if canaryWeight > c.totalWeight(canary) {
				canaryWeight = c.totalWeight(canary)
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
		if promote := c.runConfirmPromotionHooks(canary, canaryController); !promote {
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
		if err := meshRouter.SetRoutes(canary, 0, c.totalWeight(canary), false); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recorder.SetWeight(canary, 0, c.totalWeight(canary))

		if err := canaryController.SetStatusIterations(canary, canary.Status.Iterations+1); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
		c.recordEventInfof(canary, "Advance %s.%s canary iteration %v/%v",
			canary.Name, canary.Namespace, canary.Status.Iterations+1, canary.GetAnalysis().Iterations)
		return
	}

	// check promotion gate
	if promote := c.runConfirmPromotionHooks(canary, canaryController); !promote {
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
			if err := meshRouter.SetRoutes(canary, c.totalWeight(canary), 0, true); err != nil {
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
	if promote := c.runConfirmPromotionHooks(canary, canaryController); !promote {
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
			if err := meshRouter.SetRoutes(canary, 0, c.totalWeight(canary), false); err != nil {
				c.recordEventWarningf(canary, "%v", err)
				return
			}
			c.recorder.SetWeight(canary, 0, c.totalWeight(canary))
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

func (c *Controller) shouldSkipAnalysis(canary *flaggerv1.Canary, canaryController canary.Controller, meshRouter router.Interface, scalerReconciler canary.ScalerReconciler, err error, retriable bool) bool {
	if !canary.SkipAnalysis() {
		return false
	}

	// regardless if analysis is being skipped, rollback if canary failed to progress
	if !retriable {
		c.recordEventWarningf(canary, "Rolling back %s.%s progress deadline exceeded %v", canary.Name, canary.Namespace, err)
		c.alert(canary, fmt.Sprintf("Progress deadline exceeded %v", err), false, flaggerv1.SeverityError)
		c.rollback(canary, canaryController, meshRouter, scalerReconciler)

		return true
	}

	// route all traffic to primary
	primaryWeight := c.totalWeight(canary)
	canaryWeight := 0
	if err := meshRouter.SetRoutes(canary, primaryWeight, canaryWeight, false); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return true
	}
	c.recorder.SetWeight(canary, primaryWeight, canaryWeight)

	// copy spec and configs from canary to primary
	c.recordEventInfof(canary, "Copying %s.%s template spec to %s-primary.%s",
		canary.Spec.TargetRef.Name, canary.Namespace, canary.Spec.TargetRef.Name, canary.Namespace)
	if err := canaryController.Promote(canary); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return true
	}

	if scalerReconciler != nil {
		if err := scalerReconciler.ReconcilePrimaryScaler(canary, false); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return true
		}
		if err := scalerReconciler.PauseTargetScaler(canary); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return true
		}
	}

	// shutdown canary
	if err := canaryController.ScaleToZero(canary); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return true
	}

	// update status phase
	if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseSucceeded); err != nil {
		c.recordEventWarningf(canary, "%v", err)
		return true
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
	if canary.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		canary.Status.Phase == flaggerv1.CanaryPhaseWaiting ||
		canary.Status.Phase == flaggerv1.CanaryPhaseWaitingPromotion ||
		canary.Status.Phase == flaggerv1.CanaryPhasePromoting ||
		canary.Status.Phase == flaggerv1.CanaryPhaseFinalising {
		return true, nil
	}

	// Make sure to sync lastAppliedSpec even if the canary is in a failed state.
	if canary.Status.Phase == flaggerv1.CanaryPhaseFailed {
		if err := canaryController.SyncStatus(canary, canary.Status); err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
			return false, err
		}
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

func (c *Controller) checkCanaryStatus(canary *flaggerv1.Canary, canaryController canary.Controller, scalerReconciler canary.ScalerReconciler, shouldAdvance bool) bool {
	c.recorder.SetStatus(canary, canary.Status.Phase)
	if canary.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		canary.Status.Phase == flaggerv1.CanaryPhaseWaitingPromotion ||
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

	if shouldAdvance {
		// check confirm-rollout gate
		if isApproved := c.runConfirmRolloutHooks(canary, canaryController); !isApproved {
			return false
		}

		canaryPhaseProgressing := canary.DeepCopy()
		canaryPhaseProgressing.Status.Phase = flaggerv1.CanaryPhaseProgressing
		c.recordEventInfof(canaryPhaseProgressing, "New revision detected! Scaling up %s.%s", canaryPhaseProgressing.Spec.TargetRef.Name, canaryPhaseProgressing.Namespace)
		c.alert(canaryPhaseProgressing, "New revision detected, progressing canary analysis.",
			true, flaggerv1.SeverityInfo)

		if scalerReconciler != nil {
			err = scalerReconciler.ResumeTargetScaler(canary)
			if err != nil {
				c.recordEventWarningf(canary, "%v", err)
				return false
			}
		}
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
	if canary.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
		canary.Status.Phase == flaggerv1.CanaryPhaseWaitingPromotion {
		if diff, _ := canaryController.HasTargetChanged(canary); diff {
			return true
		}
		if diff, _ := canaryController.HaveDependenciesChanged(canary); diff {
			return true
		}
	}
	return false
}

func (c *Controller) rollback(canary *flaggerv1.Canary, canaryController canary.Controller,
	meshRouter router.Interface, scalerReconciler canary.ScalerReconciler) {
	if canary.Status.FailedChecks >= canary.GetAnalysisThreshold() {
		c.recordEventWarningf(canary, "Rolling back %s.%s failed checks threshold reached %v",
			canary.Name, canary.Namespace, canary.Status.FailedChecks)
		c.alert(canary, fmt.Sprintf("Failed checks threshold reached %v", canary.Status.FailedChecks),
			false, flaggerv1.SeverityError)
	}

	// route all traffic back to primary
	primaryWeight := c.totalWeight(canary)
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

	if scalerReconciler != nil {
		if err := scalerReconciler.PauseTargetScaler(canary); err != nil {
			c.recordEventWarningf(canary, "%v", err)
			return
		}
	}
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

func (c *Controller) setPhaseInitialized(cd *flaggerv1.Canary, canaryController canary.Controller) error {
	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		cd.Status.Phase = flaggerv1.CanaryPhaseInitialized
		if err := canaryController.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseInitialized}); err != nil {
			return fmt.Errorf("failed to sync canary %s.%s status: %w", cd.Name, cd.Namespace, err)
		}

		canary, err := c.flaggerClient.FlaggerV1beta1().Canaries(cd.Namespace).Get(context.TODO(), cd.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get canary %s.%s: %w", cd.Name, cd.Namespace, err)
		}
		// We need to sync the LastAppliedSpec and TrackedConfigs of the `cd` Canary object as it
		// is used later to determine whether target revision has changed in `shouldAdvance()`.
		cd.Status.LastAppliedSpec = canary.Status.LastAppliedSpec
		cd.Status.TrackedConfigs = canary.Status.TrackedConfigs

		c.recorder.SetStatus(cd, flaggerv1.CanaryPhaseInitialized)
		c.recordEventInfof(cd, "Initialization done! %s.%s", cd.Name, cd.Namespace)
		c.alert(cd, fmt.Sprintf("New %s detected, initialization completed.", cd.Spec.TargetRef.Kind),
			true, flaggerv1.SeverityInfo)
	}
	return nil
}

func (c *Controller) setPhaseInitializing(cd *flaggerv1.Canary) error {
	phase := flaggerv1.CanaryPhaseInitializing
	firstTry := true
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			cd, err = c.flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}

		if ok, conditions := canary.MakeStatusConditions(cd, phase); ok {
			cdCopy := cd.DeepCopy()
			cdCopy.Status.Conditions = conditions
			cdCopy.Status.LastTransitionTime = metav1.Now()
			cdCopy.Status.Phase = phase
			_, err = c.flaggerClient.FlaggerV1beta1().Canaries(cd.Namespace).UpdateStatus(context.TODO(), cdCopy, metav1.UpdateOptions{})
		}
		firstTry = false
		return
	})

	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}
