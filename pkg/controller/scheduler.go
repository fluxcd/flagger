package controller

import (
	"fmt"
	"time"

	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
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

		// schedule new jobs
		if _, exists := c.jobs[name]; !exists {
			job := CanaryJob{
				Name:      canary.Name,
				Namespace: canary.Namespace,
				function:  c.advanceCanary,
				done:      make(chan bool),
				ticker:    time.NewTicker(canary.GetAnalysisInterval()),
			}

			c.jobs[name] = job
			job.Start()
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
				c.logger.Errorf("Bad things will happen! Found more than one canary with the same target %s",
					targetName)
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
	cd, err := c.flaggerClient.FlaggerV1alpha3().Canaries(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("Canary %s.%s not found", name, namespace)
		return
	}

	// create primary deployment and hpa if needed
	if err := c.deployer.Sync(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// create ClusterIP services and virtual service if needed
	if err := c.router.Sync(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	if ok, err := c.deployer.ShouldAdvance(cd); !ok {
		if err != nil {
			c.recordEventWarningf(cd, "%v", err)
		}
		return
	}

	// set max weight default value to 100%
	maxWeight := 100
	if cd.Spec.CanaryAnalysis.MaxWeight > 0 {
		maxWeight = cd.Spec.CanaryAnalysis.MaxWeight
	}

	// check primary deployment status
	if _, err := c.deployer.IsPrimaryReady(cd); err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// check if virtual service exists
	// and if it contains weighted destination routes to the primary and canary services
	primaryRoute, canaryRoute, err := c.router.GetRoutes(cd)
	if err != nil {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	c.recorder.SetWeight(cd, primaryRoute.Weight, canaryRoute.Weight)

	// check if canary analysis should start (canary revision has changes) or continue
	if ok := c.checkCanaryStatus(cd, c.deployer); !ok {
		return
	}

	defer func() {
		c.recorder.SetDuration(cd, time.Since(begin))
	}()

	// check canary deployment status
	retriable, err := c.deployer.IsCanaryReady(cd)
	if err != nil && retriable {
		c.recordEventWarningf(cd, "%v", err)
		return
	}

	// check if the number of failed checks reached the threshold
	if cd.Status.Phase == flaggerv1.CanaryProgressing &&
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
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		if err := c.router.SetRoutes(cd, primaryRoute, canaryRoute); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recorder.SetWeight(cd, primaryRoute.Weight, canaryRoute.Weight)
		c.recordEventWarningf(cd, "Canary failed! Scaling down %s.%s",
			cd.Name, cd.Namespace)

		// shutdown canary
		if err := c.deployer.Scale(cd, 0); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// mark canary as failed
		if err := c.deployer.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryFailed}); err != nil {
			c.logger.Errorf("%v", err)
			return
		}

		c.recorder.SetStatus(cd)
		return
	}

	// check if the canary success rate is above the threshold
	// skip check if no traffic is routed to canary
	if canaryRoute.Weight == 0 {
		c.recordEventInfof(cd, "Starting canary deployment for %s.%s", cd.Name, cd.Namespace)
	} else {
		if ok := c.analyseCanary(cd); !ok {
			if err := c.deployer.SetFailedChecks(cd, cd.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
			return
		}
	}

	// increase canary traffic percentage
	if canaryRoute.Weight < maxWeight {
		primaryRoute.Weight -= cd.Spec.CanaryAnalysis.StepWeight
		if primaryRoute.Weight < 0 {
			primaryRoute.Weight = 0
		}
		canaryRoute.Weight += cd.Spec.CanaryAnalysis.StepWeight
		if primaryRoute.Weight > 100 {
			primaryRoute.Weight = 100
		}

		if err := c.router.SetRoutes(cd, primaryRoute, canaryRoute); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recorder.SetWeight(cd, primaryRoute.Weight, canaryRoute.Weight)
		c.recordEventInfof(cd, "Advance %s.%s canary weight %v", cd.Name, cd.Namespace, canaryRoute.Weight)

		// promote canary
		primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
		if canaryRoute.Weight == maxWeight {
			c.recordEventInfof(cd, "Copying %s.%s template spec to %s.%s",
				cd.Spec.TargetRef.Name, cd.Namespace, primaryName, cd.Namespace)
			if err := c.deployer.Promote(cd); err != nil {
				c.recordEventWarningf(cd, "%v", err)
				return
			}
		}
	} else {
		// route all traffic back to primary
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		if err := c.router.SetRoutes(cd, primaryRoute, canaryRoute); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		c.recorder.SetWeight(cd, primaryRoute.Weight, canaryRoute.Weight)
		c.recordEventInfof(cd, "Promotion completed! Scaling down %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)

		// shutdown canary
		if err := c.deployer.Scale(cd, 0); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}

		// update status
		if err := c.deployer.SetState(cd, flaggerv1.CanarySucceeded); err != nil {
			c.recordEventWarningf(cd, "%v", err)
			return
		}
		c.recorder.SetStatus(cd)
		c.sendNotification(cd, "Canary analysis completed successfully, promotion finished.",
			false, false)
	}
}

func (c *Controller) checkCanaryStatus(cd *flaggerv1.Canary, deployer CanaryDeployer) bool {
	c.recorder.SetStatus(cd)
	if cd.Status.Phase == flaggerv1.CanaryProgressing {
		return true
	}

	if cd.Status.Phase == "" {
		if err := deployer.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryInitialized}); err != nil {
			c.logger.Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(cd)
		c.recordEventInfof(cd, "Initialization done! %s.%s", cd.Name, cd.Namespace)
		c.sendNotification(cd, "New deployment detected, initialization completed.",
			true, false)
		return false
	}

	if diff, err := deployer.IsNewSpec(cd); diff {
		c.recordEventInfof(cd, "New revision detected! Scaling up %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		c.sendNotification(cd, "New revision detected, starting canary analysis.",
			true, false)
		if err = deployer.Scale(cd, 1); err != nil {
			c.recordEventErrorf(cd, "%v", err)
			return false
		}
		if err := deployer.SyncStatus(cd, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryProgressing}); err != nil {
			c.logger.Errorf("%v", err)
			return false
		}
		c.recorder.SetStatus(cd)
		return false
	}
	return false
}

func (c *Controller) analyseCanary(r *flaggerv1.Canary) bool {
	// run metrics checks
	for _, metric := range r.Spec.CanaryAnalysis.Metrics {
		if metric.Name == "istio_requests_total" {
			val, err := c.observer.GetDeploymentCounter(r.Spec.TargetRef.Name, r.Namespace, metric.Name, metric.Interval)
			if err != nil {
				c.recordEventErrorf(r, "Metrics server %s query failed: %v", c.observer.metricsServer, err)
				return false
			}
			if float64(metric.Threshold) > val {
				c.recordEventWarningf(r, "Halt %s.%s advancement success rate %.2f%% < %v%%",
					r.Name, r.Namespace, val, metric.Threshold)
				return false
			}
		}

		if metric.Name == "istio_request_duration_seconds_bucket" {
			val, err := c.observer.GetDeploymentHistogram(r.Spec.TargetRef.Name, r.Namespace, metric.Name, metric.Interval)
			if err != nil {
				c.recordEventErrorf(r, "Metrics server %s query failed: %v", c.observer.metricsServer, err)
				return false
			}
			t := time.Duration(metric.Threshold) * time.Millisecond
			if val > t {
				c.recordEventWarningf(r, "Halt %s.%s advancement request duration %v > %v",
					r.Name, r.Namespace, val, t)
				return false
			}
		}
	}

	// run external checks
	for _, webhook := range r.Spec.CanaryAnalysis.Webhooks {
		err := CallWebhook(r.Name, r.Namespace, webhook)
		if err != nil {
			c.recordEventWarningf(r, "Halt %s.%s advancement external check %s failed %v",
				r.Name, r.Namespace, webhook.Name, err)
			return false
		}
	}

	return true
}
