package canary

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// IsPrimaryReady checks the primary daemonset status and returns an error if
// the daemonset is in the middle of a rolling update
func (c *DaemonSetController) IsPrimaryReady(cd *flaggerv1.Canary) (bool, error) {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return true, fmt.Errorf("deployment %s.%s not found", primaryName, cd.Namespace)
		}
		return true, fmt.Errorf("deployment %s.%s query error %v", primaryName, cd.Namespace, err)
	}

	retriable, err := c.isDaemonSetReady(cd, primary)
	if err != nil {
		return retriable, fmt.Errorf("halt advancement %s.%s %s", primaryName, cd.Namespace, err.Error())
	}
	return true, nil
}

// IsCanaryReady checks the primary daemonset and returns an error if
// the daemonset is in the middle of a rolling update
func (c *DaemonSetController) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return true, fmt.Errorf("daemonset %s.%s not found", targetName, cd.Namespace)
		}
		return true, fmt.Errorf("daemonset %s.%s query error %v", targetName, cd.Namespace, err)
	}

	retriable, err := c.isDaemonSetReady(cd, canary)
	if err != nil {
		return retriable, fmt.Errorf("halt advancement %s.%s %s", targetName, cd.Namespace, err.Error())
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
			return false, fmt.Errorf("daemonset %s exceeded its progress deadline", cd.GetName())
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
