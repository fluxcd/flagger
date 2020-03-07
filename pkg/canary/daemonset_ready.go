package canary

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// IsPrimaryReady checks the primary daemonset status and returns an error if
// the daemonset is in the middle of a rolling update
func (c *DaemonSetController) IsPrimaryReady(cd *flaggerv1.Canary) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("daemonset %s.%s get query error: %w", primaryName, cd.Namespace, err)
	}

	_, err = c.isDaemonSetReady(cd, primary)
	if err != nil {
		return fmt.Errorf("primary daemonset %s.%s not ready: %w", primaryName, cd.Namespace, err)
	}
	return nil
}

// IsCanaryReady checks the primary daemonset and returns an error if
// the daemonset is in the middle of a rolling update
func (c *DaemonSetController) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("daemonset %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	retriable, err := c.isDaemonSetReady(cd, canary)
	if err != nil {
		return retriable, fmt.Errorf("canary damonset %s.%s not ready with retryablility: %v: %w",
			targetName, cd.Namespace, retriable, err)
	}
	return true, nil
}

// isDaemonSetReady determines if a daemonset is ready by checking the number of old version daemons
func (c *DaemonSetController) isDaemonSetReady(cd *flaggerv1.Canary, daemonSet *appsv1.DaemonSet) (bool, error) {
	if diff := daemonSet.Status.DesiredNumberScheduled - daemonSet.Status.UpdatedNumberScheduled; diff > 0 || daemonSet.Status.NumberUnavailable > 0 {
		from := cd.Status.LastTransitionTime
		delta := time.Duration(cd.GetProgressDeadlineSeconds()) * time.Second
		dl := from.Add(delta)
		if dl.Before(time.Now()) {
			return false, fmt.Errorf("exceeded its progressDeadlineSeconds: %d", cd.GetProgressDeadlineSeconds())
		} else {
			return true, fmt.Errorf(
				"waiting for rollout to finish: desiredNumberScheduled=%d, updatedNumberScheduled=%d, numberUnavailable=%d",
				daemonSet.Status.DesiredNumberScheduled,
				daemonSet.Status.UpdatedNumberScheduled,
				daemonSet.Status.NumberUnavailable,
			)
		}
	}
	return true, nil
}
