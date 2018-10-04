package controller

import (
	"fmt"
	"time"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	rolloutv1 "github.com/stefanprodan/steerer/pkg/apis/rollout/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) doRollouts() {
	c.rollouts.Range(func(key interface{}, value interface{}) bool {
		r := value.(*rolloutv1.Canary)
		if r.Spec.TargetKind == "Deployment" {
			go c.advanceDeploymentRollout(r.Name, r.Namespace)
		}
		return true
	})
}

func (c *Controller) advanceDeploymentRollout(name string, namespace string) {
	// gate stage: check if the rollout exists
	r, ok := c.getRollout(name, namespace)
	if !ok {
		return
	}

	// set max weight default value to 100%
	maxWeight := 100
	if r.Spec.CanaryAnalysis.MaxWeight > 0 {
		maxWeight = r.Spec.CanaryAnalysis.MaxWeight
	}

	// gate stage: check if canary deployment exists and is healthy
	canary, ok := c.getCanaryDeployment(r, r.Spec.Canary.Name, r.Namespace)
	if !ok {
		return
	}

	// gate stage: check if primary deployment exists and is healthy
	primary, ok := c.getDeployment(r, r.Spec.Primary.Name, r.Namespace)
	if !ok {
		return
	}

	// gate stage: check if virtual service exists
	// and if it contains weighted destination routes to the primary and canary services
	vs, primaryRoute, canaryRoute, ok := c.getVirtualService(r)
	if !ok {
		return
	}

	// gate stage: check if rollout should start (canary revision has changes) or continue
	if ok := c.checkRolloutStatus(r, canary); !ok {
		return
	}

	// gate stage: check if the number of failed checks reached the threshold
	if r.Status.State == "running" && r.Status.FailedChecks >= r.Spec.CanaryAnalysis.Threshold {
		c.recordEventWarningf(r, "Rolling back %s.%s failed checks threshold reached %v",
			r.Spec.Canary.Name, r.Namespace, r.Status.FailedChecks)

		// route all traffic back to primary
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		if ok := c.updateVirtualServiceRoutes(r, vs, primaryRoute, canaryRoute); !ok {
			return
		}

		c.recordEventWarningf(r, "Canary failed! Scaling down %s.%s",
			canary.GetName(), canary.Namespace)

		// shutdown canary
		c.scaleToZeroCanary(r)

		// mark rollout as failed
		c.updateRolloutStatus(r, "promotion-failed")
		return
	}

	// gate stage: check if the canary success rate is above the threshold
	// skip check if no traffic is routed to canary
	if canaryRoute.Weight == 0 {
		c.recordEventInfof(r, "Starting canary deployment for %s.%s", r.Name, r.Namespace)
	} else {
		if ok := c.checkDeploymentMetrics(r); !ok {
			c.updateRolloutFailedChecks(r, r.Status.FailedChecks+1)
			return
		}
	}

	// routing stage: increase canary traffic percentage
	if canaryRoute.Weight < maxWeight {
		primaryRoute.Weight -= r.Spec.CanaryAnalysis.StepWeight
		if primaryRoute.Weight < 0 {
			primaryRoute.Weight = 0
		}
		canaryRoute.Weight += r.Spec.CanaryAnalysis.StepWeight
		if primaryRoute.Weight > 100 {
			primaryRoute.Weight = 100
		}

		if ok := c.updateVirtualServiceRoutes(r, vs, primaryRoute, canaryRoute); !ok {
			return
		}

		c.recordEventInfof(r, "Advance %s.%s canary weight %v", r.Name, r.Namespace, canaryRoute.Weight)

		// promotion stage: override primary.template.spec with the canary spec
		if canaryRoute.Weight == maxWeight {
			c.recordEventInfof(r, "Copying %s.%s template spec to %s.%s",
				canary.GetName(), canary.Namespace, primary.GetName(), primary.Namespace)

			primary.Spec.Template.Spec = canary.Spec.Template.Spec
			_, err := c.kubeClient.AppsV1().Deployments(primary.Namespace).Update(primary)
			if err != nil {
				c.recordEventErrorf(r, "Updating template spec %s.%s failed: %v", primary.GetName(), primary.Namespace, err)
				return
			}
		}
	} else {
		// final stage: route all traffic back to primary
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		if ok := c.updateVirtualServiceRoutes(r, vs, primaryRoute, canaryRoute); !ok {
			return
		}

		// final stage: mark rollout as finished and scale canary to zero replicas
		c.recordEventInfof(r, "Scaling down %s.%s", canary.GetName(), canary.Namespace)
		c.scaleToZeroCanary(r)
		c.updateRolloutStatus(r, "promotion-finished")
	}
}

func (c *Controller) getRollout(name string, namespace string) (*rolloutv1.Canary, bool) {
	r, err := c.rolloutClient.SteererV1beta1().Canaries(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("Canary %s.%s not found", name, namespace)
		return nil, false
	}

	return r, true
}

func (c *Controller) checkRolloutStatus(r *rolloutv1.Canary, canary *appsv1.Deployment) bool {
	var err error
	if r.Status.State == "" {
		r.Status = rolloutv1.CanaryStatus{
			State:          "running",
			CanaryRevision: canary.ResourceVersion,
			FailedChecks:   0,
		}
		r, err = c.rolloutClient.SteererV1beta1().Canaries(r.Namespace).Update(r)
		if err != nil {
			c.logger.Errorf("Canary %s.%s status update failed: %v", r.Name, r.Namespace, err)
			return false
		}
		return true
	}

	if r.Status.State == "running" {
		return true
	}

	if r.Status.State == "promotion-finished" {
		c.setCanaryRevision(r, "finished")
		c.logger.Infof("Promotion completed! %s.%s revision %s", r.Spec.Canary.Name, r.Namespace,
			c.getDeploymentRevision(r.Spec.Canary.Name, r.Namespace))
		return false
	}

	if r.Status.State == "promotion-failed" {
		c.setCanaryRevision(r, "failed")
		c.logger.Infof("Promotion failed! %s.%s revision %s", r.Spec.Canary.Name, r.Namespace,
			c.getDeploymentRevision(r.Spec.Canary.Name, r.Namespace))
		return false
	}

	if r.Status.CanaryRevision != canary.ResourceVersion {
		c.recordEventInfof(r, "New revision detected %s.%s old %s new %s",
			canary.GetName(), canary.Namespace, r.Status.CanaryRevision, canary.ResourceVersion)
		canary.Spec.Replicas = int32p(1)
		canary, err = c.kubeClient.AppsV1().Deployments(canary.Namespace).Update(canary)
		if err != nil {
			c.recordEventErrorf(r, "Scaling up %s.%s failed: %v", canary.GetName(), canary.Namespace, err)
			return false
		}
		r.Status = rolloutv1.CanaryStatus{
			State:          "running",
			CanaryRevision: canary.ResourceVersion,
			FailedChecks:   0,
		}
		r, err = c.rolloutClient.SteererV1beta1().Canaries(r.Namespace).Update(r)
		if err != nil {
			c.logger.Errorf("Canary %s.%s status update failed: %v", r.Name, r.Namespace, err)
			return false
		}
		c.recordEventInfof(r, "Scaling up %s.%s", canary.GetName(), canary.Namespace)

		return false
	}

	return false
}

func (c *Controller) updateRolloutStatus(r *rolloutv1.Canary, status string) bool {
	var err error
	r.Status.State = status
	r, err = c.rolloutClient.SteererV1beta1().Canaries(r.Namespace).Update(r)
	if err != nil {
		c.logger.Errorf("Canary %s.%s status update failed: %v", r.Name, r.Namespace, err)
		return false
	}
	return true
}

func (c *Controller) updateRolloutFailedChecks(r *rolloutv1.Canary, val int) bool {
	var err error
	r.Status.FailedChecks = val
	r, err = c.rolloutClient.SteererV1beta1().Canaries(r.Namespace).Update(r)
	if err != nil {
		c.logger.Errorf("Canary %s.%s status update failed: %v", r.Name, r.Namespace, err)
		return false
	}
	return true
}

func (c *Controller) getDeployment(r *rolloutv1.Canary, name string, namespace string) (*appsv1.Deployment, bool) {
	dep, err := c.kubeClient.AppsV1().Deployments(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.recordEventErrorf(r, "Deployment %s.%s not found", name, namespace)
		return nil, false
	}

	if msg, healthy := getDeploymentStatus(dep); !healthy {
		c.recordEventWarningf(r, "Halt %s.%s advancement %s", dep.GetName(), dep.Namespace, msg)
		return nil, false
	}

	if dep.Spec.Replicas == nil || *dep.Spec.Replicas == 0 {
		return nil, false
	}

	return dep, true
}

func (c *Controller) getCanaryDeployment(r *rolloutv1.Canary, name string, namespace string) (*appsv1.Deployment, bool) {
	dep, err := c.kubeClient.AppsV1().Deployments(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.recordEventErrorf(r, "Deployment %s.%s not found", name, namespace)
		return nil, false
	}

	if msg, healthy := getDeploymentStatus(dep); !healthy {
		c.recordEventWarningf(r, "Halt %s.%s advancement %s", dep.GetName(), dep.Namespace, msg)
		return nil, false
	}

	return dep, true
}

func (c *Controller) getDeploymentRevision(name string, namespace string) string {
	dep, err := c.kubeClient.AppsV1().Deployments(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("Deployment %s.%s not found", name, namespace)
		return ""
	}

	return dep.ResourceVersion
}

func (c *Controller) checkDeploymentMetrics(r *rolloutv1.Canary) bool {
	for _, metric := range r.Spec.CanaryAnalysis.Metrics {
		if metric.Name == "istio_requests_total" {
			val, err := c.getDeploymentCounter(r.Spec.Canary.Name, r.Namespace, metric.Name, metric.Interval)
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
			val, err := c.GetDeploymentHistogram(r.Spec.Canary.Name, r.Namespace, metric.Name, metric.Interval)
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

func (c *Controller) scaleToZeroCanary(r *rolloutv1.Canary) {
	canary, err := c.kubeClient.AppsV1().Deployments(r.Namespace).Get(r.Spec.Canary.Name, v1.GetOptions{})
	if err != nil {
		c.recordEventErrorf(r, "Deployment %s.%s not found", r.Spec.Canary.Name, r.Namespace)
		return
	}
	//HPA https://github.com/kubernetes/kubernetes/pull/29212
	canary.Spec.Replicas = int32p(0)
	canary, err = c.kubeClient.AppsV1().Deployments(canary.Namespace).Update(canary)
	if err != nil {
		c.recordEventErrorf(r, "Scaling down %s.%s failed: %v", canary.GetName(), canary.Namespace, err)
		return
	}
}

func (c *Controller) setCanaryRevision(r *rolloutv1.Canary, status string) {
	canaryRevision := c.getDeploymentRevision(r.Spec.Canary.Name, r.Namespace)
	r, ok := c.getRollout(r.Name, r.Namespace)
	if !ok {
		return
	}
	r.Status = rolloutv1.CanaryStatus{
		State:          status,
		CanaryRevision: canaryRevision,
		FailedChecks:   r.Status.FailedChecks,
	}
	r, err := c.rolloutClient.SteererV1beta1().Canaries(r.Namespace).Update(r)
	if err != nil {
		c.logger.Errorf("Canary %s.%s status update failed: %v", r.Name, r.Namespace, err)
	}
	//c.logger.Infof("Canary %s.%s status %+v", r.Spec.Canary.Name, r.Namespace, r.Status)
}

func (c *Controller) getVirtualService(r *rolloutv1.Canary) (
	vs *istiov1alpha3.VirtualService,
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
	ok bool,
) {
	var err error
	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(r.Namespace).Get(r.Spec.VirtualService.Name, v1.GetOptions{})
	if err != nil {
		c.recordEventErrorf(r, "VirtualService %s.%s not found", r.Spec.VirtualService.Name, r.Namespace)
		return
	}

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == r.Spec.Primary.Host {
				primary = route
			}
			if route.Destination.Host == r.Spec.Canary.Host {
				canary = route
			}
		}
	}

	if primary.Weight == 0 && canary.Weight == 0 {
		c.recordEventErrorf(r, "VirtualService %s.%s does not contain routes for %s and %s",
			r.Spec.VirtualService.Name, r.Namespace, r.Spec.Primary.Host, r.Spec.Canary.Host)
		return
	}

	ok = true
	return
}

func (c *Controller) updateVirtualServiceRoutes(
	r *rolloutv1.Canary,
	vs *istiov1alpha3.VirtualService,
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
) bool {
	vs.Spec.Http = []istiov1alpha3.HTTPRoute{
		{
			Route: []istiov1alpha3.DestinationWeight{primary, canary},
		},
	}

	var err error
	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(r.Namespace).Update(vs)
	if err != nil {
		c.recordEventErrorf(r, "VirtualService %s.%s update failed: %v", r.Spec.VirtualService.Name, r.Namespace, err)
		return false
	}
	return true
}

func getDeploymentStatus(deployment *appsv1.Deployment) (string, bool) {
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		cond := getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
		if cond != nil && cond.Reason == "ProgressDeadlineExceeded" {
			return fmt.Sprintf("deployment %q exceeded its progress deadline", deployment.GetName()), false
		} else if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return fmt.Sprintf("waiting for rollout to finish: %d out of %d new replicas have been updated",
				deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false
		} else if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("waiting for rollout to finish: %d old replicas are pending termination",
				deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false
		} else if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("waiting for rollout to finish: %d of %d updated replicas are available",
				deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false
		}
	} else {
		return "waiting for rollout to finish: observed deployment generation less then desired generation", false
	}

	return "ready", true
}

func getDeploymentCondition(
	status appsv1.DeploymentStatus,
	conditionType appsv1.DeploymentConditionType,
) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}

func int32p(i int32) *int32 {
	return &i
}
