package controller

import (
	"fmt"
	"time"

	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) scheduleCanaries() {
	c.rollouts.Range(func(key interface{}, value interface{}) bool {
		r := value.(*flaggerv1.Canary)
		if r.Spec.TargetRef.Kind == "Deployment" {
			go c.advanceCanary(r.Name, r.Namespace)
		}
		return true
	})
}

func (c *Controller) advanceCanary(name string, namespace string) {
	// check if the rollout exists
	r, err := c.rolloutClient.FlaggerV1alpha1().Canaries(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("Canary %s.%s not found", name, namespace)
		return
	}

	deployer := CanaryDeployer{
		logger:        c.logger,
		kubeClient:    c.kubeClient,
		istioClient:   c.istioClient,
		flaggerClient: c.rolloutClient,
	}

	// create primary deployment and hpa if needed
	err = deployer.Sync(r)
	if err != nil {
		c.recordEventWarningf(r, "%v", err)
		return
	}

	router := CanaryRouter{
		logger:        c.logger,
		kubeClient:    c.kubeClient,
		istioClient:   c.istioClient,
		flaggerClient: c.rolloutClient,
	}

	// create ClusterIP services and virtual service if needed
	err = router.Sync(r)
	if err != nil {
		c.recordEventWarningf(r, "%v", err)
		return
	}

	// set max weight default value to 100%
	maxWeight := 100
	if r.Spec.CanaryAnalysis.MaxWeight > 0 {
		maxWeight = r.Spec.CanaryAnalysis.MaxWeight
	}

	// check primary and canary deployments status
	err = deployer.IsDeploymentHealthy(r)
	if err != nil {
		c.recordEventWarningf(r, "%v", err)
		return
	}

	// check if virtual service exists
	// and if it contains weighted destination routes to the primary and canary services
	primaryRoute, canaryRoute, err := router.GetRoutes(r)
	if err != nil {
		c.recordEventWarningf(r, "%v", err)
		return
	}

	// check if canary analysis should start (canary revision has changes) or continue
	if ok := c.checkCanaryStatus(r, deployer); !ok {
		return
	}

	// check if the number of failed checks reached the threshold
	if r.Status.State == "running" && r.Status.FailedChecks >= r.Spec.CanaryAnalysis.Threshold {
		c.recordEventWarningf(r, "Rolling back %s.%s failed checks threshold reached %v",
			r.Name, r.Namespace, r.Status.FailedChecks)

		// route all traffic back to primary
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		if err := router.SetRoutes(r, primaryRoute, canaryRoute); err != nil {
			c.recordEventWarningf(r, "%v", err)
			return
		}

		c.recordEventWarningf(r, "Canary failed! Scaling down %s.%s",
			r.Spec.TargetRef.Name, r.Namespace)

		// shutdown canary
		err = deployer.Scale(r, 0)
		if err != nil {
			c.recordEventWarningf(r, "%v", err)
			return
		}

		// mark canary as failed
		err := deployer.SetState(r, "failed")
		if err != nil {
			c.logger.Errorf("%v", err)
			return
		}
		return
	}

	// check if the canary success rate is above the threshold
	// skip check if no traffic is routed to canary
	if canaryRoute.Weight == 0 {
		c.recordEventInfof(r, "Starting canary deployment for %s.%s", r.Name, r.Namespace)
	} else {
		if ok := c.checkCanaryMetrics(r); !ok {
			if err = deployer.SetFailedChecks(r, r.Status.FailedChecks+1); err != nil {
				c.recordEventWarningf(r, "%v", err)
				return
			}
			return
		}
	}

	// increase canary traffic percentage
	if canaryRoute.Weight < maxWeight {
		primaryRoute.Weight -= r.Spec.CanaryAnalysis.StepWeight
		if primaryRoute.Weight < 0 {
			primaryRoute.Weight = 0
		}
		canaryRoute.Weight += r.Spec.CanaryAnalysis.StepWeight
		if primaryRoute.Weight > 100 {
			primaryRoute.Weight = 100
		}

		if err = router.SetRoutes(r, primaryRoute, canaryRoute); err != nil {
			c.recordEventWarningf(r, "%v", err)
			return
		}

		c.recordEventInfof(r, "Advance %s.%s canary weight %v", r.Name, r.Namespace, canaryRoute.Weight)

		// promote canary
		primaryName := fmt.Sprintf("%s-primary", r.Spec.TargetRef.Name)
		if canaryRoute.Weight == maxWeight {
			c.recordEventInfof(r, "Copying %s.%s template spec to %s.%s",
				r.Spec.TargetRef.Name, r.Namespace, primaryName, r.Namespace)

			err := deployer.Promote(r)
			if err != nil {
				c.recordEventWarningf(r, "%v", err)
				return
			}
		}
	} else {
		// route all traffic back to primary
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		if err = router.SetRoutes(r, primaryRoute, canaryRoute); err != nil {
			c.recordEventWarningf(r, "%v", err)
			return
		}

		c.recordEventInfof(r, "Promotion completed! Scaling down %s.%s", r.Spec.TargetRef.Name, r.Namespace)

		// shutdown canary
		err = deployer.Scale(r, 0)
		if err != nil {
			c.recordEventWarningf(r, "%v", err)
			return
		}

		// update status
		err = deployer.SetState(r, "finished")
		if err != nil {
			c.recordEventWarningf(r, "%v", err)
			return
		}
	}
}

func (c *Controller) checkCanaryStatus(r *flaggerv1.Canary, deployer CanaryDeployer) bool {
	if r.Status.State == "running" {
		return true
	}

	if r.Status.State == "" {
		status := flaggerv1.CanaryStatus{
			State:        "initialized",
			FailedChecks: 0,
		}

		err := deployer.SyncStatus(r, status)
		if err != nil {
			c.logger.Errorf("%v", err)
			return false
		}

		c.recordEventInfof(r, "Initialization done! %s.%s", r.Name, r.Namespace)
		return false
	}

	if diff, err := deployer.IsNewSpec(r); diff {
		c.recordEventInfof(r, "New revision detected %s.%s", r.Spec.TargetRef.Name, r.Namespace)
		err = deployer.Scale(r, 1)
		if err != nil {
			c.recordEventErrorf(r, "%v", err)
			return false
		}

		status := flaggerv1.CanaryStatus{
			State:        "running",
			FailedChecks: 0,
		}
		err := deployer.SyncStatus(r, status)
		if err != nil {
			c.logger.Errorf("%v", err)
			return false
		}
		c.recordEventInfof(r, "Scaling up %s.%s", r.Spec.TargetRef.Name, r.Namespace)

		return false
	}

	return false
}

func (c *Controller) checkCanaryMetrics(r *flaggerv1.Canary) bool {
	observer := &CanaryObserver{
		metricsServer: c.metricsServer,
	}
	for _, metric := range r.Spec.CanaryAnalysis.Metrics {
		if metric.Name == "istio_requests_total" {
			val, err := observer.GetDeploymentCounter(r.Spec.TargetRef.Name, r.Namespace, metric.Name, metric.Interval)
			if err != nil {
				c.recordEventErrorf(r, "Metrics server %s query failed: %v", c.metricsServer, err)
				return false
			}
			if float64(metric.Threshold) > val {
				c.recordEventWarningf(r, "Halt %s.%s advancement success rate %.2f%% < %v%%",
					r.Name, r.Namespace, val, metric.Threshold)
				return false
			}
		}

		if metric.Name == "istio_request_duration_seconds_bucket" {
			val, err := observer.GetDeploymentHistogram(r.Spec.TargetRef.Name, r.Namespace, metric.Name, metric.Interval)
			if err != nil {
				c.recordEventErrorf(r, "Metrics server %s query failed: %v", c.metricsServer, err)
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

	return true
}
