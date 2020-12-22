package canary

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// IsPrimaryReady checks the primary deployment status and returns an error if
// the deployment is in the middle of a rolling update or if the pods are unhealthy
// it will return a non retryable error if the rolling update is stuck
func (c *DeploymentController) IsPrimaryReady(cd *flaggerv1.Canary) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s get query error: %w", primaryName, cd.Namespace, err)
	}

	_, err = c.isDeploymentReady(primary, cd.GetProgressDeadlineSeconds())
	if err != nil {
		return fmt.Errorf("%s.%s not ready: %w", primaryName, cd.Namespace, err)
	}

	if primary.Spec.Replicas == int32p(0) {
		return fmt.Errorf("halt %s.%s advancement: primary deployment is scaled to zero",
			cd.Name, cd.Namespace)
	}
	return nil
}

// IsCanaryReady checks the canary deployment status and returns an error if
// the deployment is in the middle of a rolling update or if the pods are unhealthy
// it will return a non retriable error if the rolling update is stuck
func (c *DeploymentController) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	retryable, err := c.isDeploymentReady(canary, cd.GetProgressDeadlineSeconds())
	if err != nil {
		return retryable, fmt.Errorf(
			"canary deployment %s.%s not ready: %w",
			targetName, cd.Namespace, err,
		)
	}
	return true, nil
}

// isDeploymentReady determines if a deployment is ready by checking the status conditions
// if a deployment has exceeded the progress deadline it returns a non retriable error
func (c *DeploymentController) isDeploymentReady(deployment *appsv1.Deployment, deadline int) (bool, error) {
	retriable := true
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		progress := c.getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
		if progress != nil {
			// Determine if the deployment is stuck by checking if there is a minimum replicas unavailable condition
			// and if the last update time exceeds the deadline
			available := c.getDeploymentCondition(deployment.Status, appsv1.DeploymentAvailable)
			if available != nil && available.Status == "False" && available.Reason == "MinimumReplicasUnavailable" {
				from := available.LastUpdateTime
				delta := time.Duration(deadline) * time.Second
				retriable = !from.Add(delta).Before(time.Now())
			}
		}

		if progress != nil && progress.Reason == "ProgressDeadlineExceeded" {
			return false, fmt.Errorf("deployment %q exceeded its progress deadline", deployment.GetName())
		} else if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d out of %d new replicas have been updated",
				deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas)
		} else if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d old replicas are pending termination",
				deployment.Status.Replicas-deployment.Status.UpdatedReplicas)
		} else if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d of %d updated replicas are available",
				deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas)
		}
	} else {
		return true, fmt.Errorf(
			"waiting for rollout to finish: observed deployment generation less then desired generation")
	}
	return true, nil
}

func (c *DeploymentController) getDeploymentCondition(
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
