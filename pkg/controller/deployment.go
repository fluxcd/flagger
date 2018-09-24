package controller

import (
	"fmt"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	rolloutv1 "github.com/stefanprodan/steerer/pkg/apis/rollout/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	revisionAnnotation = "apps.weave.works/canary-revision"
	statusAnnotation   = "apps.weave.works/status"
)

func (c *Controller) doRollout() {
	c.rollouts.Range(func(key interface{}, value interface{}) bool {
		r := value.(*rolloutv1.Rollout)
		if r.Spec.TargetKind == "Deployment" {
			go c.advanceDeploymentRollout(r.Name, r.Namespace)
		}
		return true
	})
}

func (c *Controller) advanceDeploymentRollout(name string, namespace string) {
	r, ok := c.getRollout(name, namespace)
	if !ok {
		return
	}

	primary, ok := c.getDeployment(r.Spec.Primary.Name, r.Namespace)
	if !ok {
		return
	}

	canary, ok := c.getDeployment(r.Spec.Canary.Name, r.Namespace)
	if !ok {
		return
	}

	vs, primaryRoute, canaryRoute, ok := c.getVirtualService(r)
	if !ok {
		return
	}

	if ok := c.updateRolloutAnnotations(r, canary.ResourceVersion); !ok {
		return
	}

	// skip HTTP error rate check when no traffic is routed to canary
	if canaryRoute.Weight == 0 {
		c.recordEventInfof(r,"Stating rollout for %s.%s", r.Name, r.Namespace)
	} else {
		if ok := c.checkDeploymentSuccessRate(r); !ok {
			return
		}
	}

	if canaryRoute.Weight != 100 {
		primaryRoute.Weight -= 10
		canaryRoute.Weight += 10
		vs.Spec.Http = []istiov1alpha3.HTTPRoute{
			{
				Route: []istiov1alpha3.DestinationWeight{primaryRoute, canaryRoute},
			},
		}

		_, err := c.istioClient.NetworkingV1alpha3().VirtualServices(r.Namespace).Update(vs)
		if err != nil {
			c.recordEventErrorf(r,"VirtualService %s.%s update failed: %v", r.Spec.VirtualService.Name, r.Namespace, err)
			return
		} else {
			c.recordEventInfof(r,"Advance rollout %s.%s weight %v", r.Name, r.Namespace, canaryRoute.Weight)
		}

		if canaryRoute.Weight == 100 {
			c.recordEventInfof(r,"Copying %s.%s template spec to %s.%s",
				canary.GetName(), canary.Namespace, primary.GetName(), primary.Namespace)

			primary.Spec.Template.Spec = canary.Spec.Template.Spec
			_, err = c.kubeClient.AppsV1().Deployments(primary.Namespace).Update(primary)
			if err != nil {
				c.recordEventErrorf(r,"Deployment %s.%s promotion failed: %v", primary.GetName(), primary.Namespace, err)
				return
			}
		}
	} else {
		primaryRoute.Weight = 100
		canaryRoute.Weight = 0
		vs.Spec.Http = []istiov1alpha3.HTTPRoute{
			{
				Route: []istiov1alpha3.DestinationWeight{primaryRoute, canaryRoute},
			},
		}
		vs.Annotations[statusAnnotation] = "finished"
		_, err := c.istioClient.NetworkingV1alpha3().VirtualServices(r.Namespace).Update(vs)
		if err != nil {
			c.recordEventErrorf(r,"VirtualService %s.%s annotations update failed: %v", r.Spec.VirtualService.Name, r.Namespace, err)
			return
		}
		c.recordEventInfof(r,"%s.%s promotion complete! Scaling down %s.%s",
			r.Name, r.Namespace, canary.GetName(), canary.Namespace)
		c.scaleToZeroCanary(r)
	}
}

func (c *Controller) scaleToZeroCanary(r *rolloutv1.Rollout) {
	canary, err := c.kubeClient.AppsV1().Deployments(r.Namespace).Get(r.Spec.Canary.Name, v1.GetOptions{})
	if err != nil {
		c.recordEventErrorf(r,"Deployment %s.%s not found", r.Spec.Canary.Name, r.Namespace)
		return
	}
	//HPA https://github.com/kubernetes/kubernetes/pull/29212
	canary.Spec.Replicas = int32p(0)
	_, err = c.kubeClient.AppsV1().Deployments(canary.Namespace).Update(canary)
	if err != nil {
		c.recordEventErrorf(r,"Scaling down %s.%s failed: %v", canary.GetName(), canary.Namespace, err)
		return
	}
}

func (c *Controller) checkDeploymentSuccessRate(r *rolloutv1.Rollout) bool {
	val, err := c.getDeploymentMetric(r.Spec.Canary.Name, r.Namespace, r.Spec.Metric.Name, r.Spec.Metric.Interval)
	if err != nil {
		c.recordEventErrorf(r,"Metric query error: %v", err)
		return false
	}

	if float64(r.Spec.Metric.Threshold) > val {
		c.recordEventErrorf(r,"Halt rollout %s.%s success rate %.2f%% < %v%%",
			r.Name, r.Namespace, val, r.Spec.Metric.Threshold)
		return false
	}

	return true
}

func (c *Controller) updateRolloutAnnotations(r *rolloutv1.Rollout, canaryVersion string) bool {
	if val, ok := r.Annotations[revisionAnnotation]; !ok {
		var err error
		r.Annotations[revisionAnnotation] = canaryVersion
		r.Annotations[statusAnnotation] = "running"
		r, err = c.rolloutClient.AppsV1beta1().Rollouts(r.Namespace).Update(r)
		if err != nil {
			c.recordEventErrorf(r,"Rollout %s.%s annotations update failed: %v", r.Name, r.Namespace, err)
			return false
		}
		return true
	} else {
		if r.Annotations[statusAnnotation] == "running" {
			return true
		}
		if val != canaryVersion {
			var err error
			r.Annotations[revisionAnnotation] = canaryVersion
			r.Annotations[statusAnnotation] = "running"
			r, err = c.rolloutClient.AppsV1beta1().Rollouts(r.Namespace).Update(r)
			if err != nil {
				c.recordEventErrorf(r,"Rollout %s.%s annotations update failed: %v", r.Name, r.Namespace, err)
				return false
			}
			return true
		}
	}

	return false
}

func (c *Controller) getRollout(name string, namespace string) (*rolloutv1.Rollout, bool) {
	r, err := c.rolloutClient.AppsV1beta1().Rollouts(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("Rollout %s.%s not found", name, namespace)
		return nil, false
	}

	return r, true
}

func (c *Controller) getDeployment(name string, namespace string) (*appsv1.Deployment, bool) {
	dep, err := c.kubeClient.AppsV1().Deployments(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("Deployment %s.%s not found", name, namespace)
		return nil, false
	}

	if msg, healthy := getDeploymentStatus(dep); !healthy {
		c.logger.Infof("Halt rollout for %s.%s %s", dep.GetName(), dep.Namespace, msg)
		return nil, false
	}

	if dep.Spec.Replicas == nil || *dep.Spec.Replicas == 0 {
		return nil, false
	}

	return dep, true
}

func (c *Controller) getVirtualService(r *rolloutv1.Rollout) (
	vs *istiov1alpha3.VirtualService,
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
	ok bool,
) {
	var err error
	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(r.Namespace).Get(r.Spec.VirtualService.Name, v1.GetOptions{})
	if err != nil {
		c.logger.Errorf("VirtualService %s.%s not found", r.Spec.VirtualService.Name, r.Namespace)
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
		c.logger.Errorf("VirtualService %s.%s does not contain routes for %s and %s",
			r.Spec.VirtualService.Name, r.Namespace, r.Spec.Primary.Host, r.Spec.Canary.Host)
		return
	}

	ok = true
	return
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

func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

func int32p(i int32) *int32 {
	return &i
}
