package canary

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

func syncCanaryStatus(flaggerClient clientset.Interface, cd *flaggerv1.Canary, status flaggerv1.CanaryStatus, canaryResource interface{}, setAll func(cdCopy *flaggerv1.Canary)) error {
	hash := computeHash(canaryResource)

	firstTry := true
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			cd, err = flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}

		cdCopy := cd.DeepCopy()
		cdCopy.Status.Phase = status.Phase
		cdCopy.Status.CanaryWeight = status.CanaryWeight
		cdCopy.Status.FailedChecks = status.FailedChecks
		cdCopy.Status.Iterations = status.Iterations
		cdCopy.Status.LastAppliedSpec = hash
		cdCopy.Status.LastTransitionTime = metav1.Now()
		setAll(cdCopy)

		if ok, conditions := MakeStatusConditions(cd, status.Phase); ok {
			cdCopy.Status.Conditions = conditions
		}

		err = updateStatusWithUpgrade(flaggerClient, cdCopy)
		firstTry = false
		return
	})

	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

func setStatusFailedChecks(flaggerClient clientset.Interface, cd *flaggerv1.Canary, val int) error {
	firstTry := true
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			cd, err = flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}
		cdCopy := cd.DeepCopy()
		cdCopy.Status.FailedChecks = val
		cdCopy.Status.LastTransitionTime = metav1.Now()

		err = updateStatusWithUpgrade(flaggerClient, cdCopy)
		firstTry = false
		return
	})
	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

func setStatusWeight(flaggerClient clientset.Interface, cd *flaggerv1.Canary, val int) error {
	firstTry := true
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			cd, err = flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}
		cdCopy := cd.DeepCopy()
		cdCopy.Status.CanaryWeight = val
		cdCopy.Status.LastTransitionTime = metav1.Now()

		err = updateStatusWithUpgrade(flaggerClient, cdCopy)
		firstTry = false
		return
	})
	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

func setStatusIterations(flaggerClient clientset.Interface, cd *flaggerv1.Canary, val int) error {
	firstTry := true
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			cd, err = flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}

		cdCopy := cd.DeepCopy()
		cdCopy.Status.Iterations = val
		cdCopy.Status.LastTransitionTime = metav1.Now()

		err = updateStatusWithUpgrade(flaggerClient, cdCopy)
		firstTry = false
		return
	})

	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

func setStatusPhase(flaggerClient clientset.Interface, cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	firstTry := true
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			cd, err = flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}

		cdCopy := cd.DeepCopy()
		cdCopy.Status.Phase = phase
		cdCopy.Status.LastTransitionTime = metav1.Now()

		if phase != flaggerv1.CanaryPhaseProgressing && phase != flaggerv1.CanaryPhaseWaiting {
			cdCopy.Status.CanaryWeight = 0
			cdCopy.Status.Iterations = 0
		}

		// on promotion set primary spec hash
		if phase == flaggerv1.CanaryPhaseInitialized || phase == flaggerv1.CanaryPhaseSucceeded {
			cdCopy.Status.LastPromotedSpec = cd.Status.LastAppliedSpec
		}

		if ok, conditions := MakeStatusConditions(cdCopy, phase); ok {
			cdCopy.Status.Conditions = conditions
		}

		err = updateStatusWithUpgrade(flaggerClient, cdCopy)
		firstTry = false
		return
	})
	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

// getStatusCondition returns a condition based on type
func getStatusCondition(status flaggerv1.CanaryStatus, conditionType flaggerv1.CanaryConditionType) *flaggerv1.CanaryCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}

// MakeStatusCondition updates the canary status conditions based on canary phase
func MakeStatusConditions(cd *flaggerv1.Canary,
	phase flaggerv1.CanaryPhase) (bool, []flaggerv1.CanaryCondition) {
	currentCondition := getStatusCondition(cd.Status, flaggerv1.PromotedType)

	message := fmt.Sprintf("New %s detected, starting initialization.", cd.Spec.TargetRef.Kind)
	status := corev1.ConditionUnknown
	switch phase {
	case flaggerv1.CanaryPhaseInitializing:
		status = corev1.ConditionUnknown
		message = fmt.Sprintf("New %s detected, starting initialization.", cd.Spec.TargetRef.Kind)
	case flaggerv1.CanaryPhaseInitialized:
		status = corev1.ConditionTrue
		message = fmt.Sprintf("%s initialization completed.", cd.Spec.TargetRef.Kind)
	case flaggerv1.CanaryPhaseWaiting:
		status = corev1.ConditionUnknown
		message = "Waiting for approval."
	case flaggerv1.CanaryPhaseProgressing:
		status = corev1.ConditionUnknown
		message = "New revision detected, starting canary analysis."
	case flaggerv1.CanaryPhasePromoting:
		status = corev1.ConditionUnknown
		message = "Canary analysis completed, starting primary rolling update."
	case flaggerv1.CanaryPhaseFinalising:
		status = corev1.ConditionUnknown
		message = "Canary analysis completed, routing all traffic to primary."
	case flaggerv1.CanaryPhaseSucceeded:
		status = corev1.ConditionTrue
		message = "Canary analysis completed successfully, promotion finished."
	case flaggerv1.CanaryPhaseFailed:
		status = corev1.ConditionFalse
		message = fmt.Sprintf("Canary analysis failed, %s scaled to zero.", cd.Spec.TargetRef.Kind)
	}

	newCondition := &flaggerv1.CanaryCondition{
		Type:               flaggerv1.PromotedType,
		Status:             status,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Message:            message,
		Reason:             string(phase),
	}

	if currentCondition != nil &&
		currentCondition.Status == newCondition.Status &&
		currentCondition.Reason == newCondition.Reason {
		return false, nil
	}

	if currentCondition != nil && currentCondition.Status == newCondition.Status {
		newCondition.LastTransitionTime = currentCondition.LastTransitionTime
	}

	return true, []flaggerv1.CanaryCondition{*newCondition}
}

// updateStatusWithUpgrade tries to update the status sub-resource
// if the status update fails with:
// Canary.flagger.app is invalid: apiVersion: Invalid value: flagger.app/v1alpha3: must be flagger.app/v1beta1
// then the canary object will be updated to the latest API version
func updateStatusWithUpgrade(flaggerClient clientset.Interface, cd *flaggerv1.Canary) error {
	_, err := flaggerClient.FlaggerV1beta1().Canaries(cd.Namespace).UpdateStatus(context.TODO(), cd, metav1.UpdateOptions{})
	if err != nil && strings.Contains(err.Error(), "flagger.app/v1alpha") {
		// upgrade alpha resource
		if _, updateErr := flaggerClient.FlaggerV1beta1().Canaries(cd.Namespace).Update(context.TODO(), cd, metav1.UpdateOptions{}); updateErr != nil {
			return fmt.Errorf("updating canary %s.%s from v1alpha to v1beta failed: %w", cd.Name, cd.Namespace, updateErr)
		}
		// retry status update
		_, err = flaggerClient.FlaggerV1beta1().Canaries(cd.Namespace).UpdateStatus(context.TODO(), cd, metav1.UpdateOptions{})
	}

	if err != nil {
		return fmt.Errorf("updating canary %s.%s status failed: %w", cd.Name, cd.Namespace, err)
	}
	return err
}
