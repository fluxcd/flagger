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

package canary

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// IsPrimaryReady checks the primary StatefulSet status and returns an error if
// the StatefulSet is in the middle of a rolling update or if the pods are unhealthy
// it will return a non retryable error if the rolling update is stuck
func (c *StatefulSetController) IsPrimaryReady(cd *flaggerv1.Canary) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primary, err := c.kubeClient.AppsV1().StatefulSets(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("StatefulSet %s.%s get query error: %w", primaryName, cd.Namespace, err)
	}

	_, err = c.isStatefulSetReady(primary, cd.GetProgressDeadlineSeconds(), cd.GetAnalysisPrimaryReadyThreshold())
	if err != nil {
		return fmt.Errorf("%s.%s not ready: %w", primaryName, cd.Namespace, err)
	}

	if primary.Spec.Replicas == int32p(0) {
		return fmt.Errorf("halt %s.%s advancement: primary StatefulSet is scaled to zero",
			cd.Name, cd.Namespace)
	}
	return nil
}

// IsCanaryReady checks the canary StatefulSet status and returns an error if
// the StatefulSet is in the middle of a rolling update or if the pods are unhealthy
// it will return a non retriable error if the rolling update is stuck
func (c *StatefulSetController) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().StatefulSets(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("StatefulSet %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	retryable, err := c.isStatefulSetReady(canary, cd.GetProgressDeadlineSeconds(), cd.GetAnalysisCanaryReadyThreshold())
	if err != nil {
		return retryable, fmt.Errorf(
			"canary StatefulSet %s.%s not ready: %w",
			targetName, cd.Namespace, err,
		)
	}
	return true, nil
}

// isStatefulSetReady determines if a StatefulSet is ready by checking the status conditions
// if a StatefulSet has exceeded the progress deadline it returns a non retriable error
func (c *StatefulSetController) isStatefulSetReady(StatefulSet *appsv1.StatefulSet, deadline int, readyThreshold int) (bool, error) {
	retriable := true
	if StatefulSet.Generation <= StatefulSet.Status.ObservedGeneration {
		readyThresholdRatio := float32(readyThreshold) / float32(100)
		readyThresholdUpdatedReplicas := int32(float32(StatefulSet.Status.UpdatedReplicas) * readyThresholdRatio)

		if StatefulSet.Spec.Replicas != nil && StatefulSet.Status.UpdatedReplicas < *StatefulSet.Spec.Replicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d out of %d new replicas have been updated",
				StatefulSet.Status.UpdatedReplicas, *StatefulSet.Spec.Replicas)
		} else if StatefulSet.Status.Replicas > StatefulSet.Status.UpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d old replicas are pending termination",
				StatefulSet.Status.Replicas-StatefulSet.Status.UpdatedReplicas)
		} else if StatefulSet.Status.AvailableReplicas < readyThresholdUpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d of %d (readyThreshold %d%%) updated replicas are available",
				StatefulSet.Status.AvailableReplicas, readyThresholdUpdatedReplicas, readyThreshold)
		}
	} else {
		return true, fmt.Errorf(
			"waiting for rollout to finish: observed StatefulSet generation less than desired generation")
	}
	return true, nil
}

func (c *StatefulSetController) getStatefulSetCondition(
	status appsv1.StatefulSetStatus,
	conditionType appsv1.StatefulSetConditionType,
) *appsv1.StatefulSetCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}
