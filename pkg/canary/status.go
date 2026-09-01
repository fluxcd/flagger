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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

func syncCanaryStatus(flaggerClient clientset.Interface, cd *flaggerv1.Canary, status flaggerv1.CanaryStatus, snap targetSnapshot, setAll func(cdCopy *flaggerv1.Canary)) error {
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
		cdCopy.Status.LastAppliedSpec = snap.hash
		cdCopy.Status.LastTrackedRevision = snap.fence
		if status.Phase == flaggerv1.CanaryPhaseInitialized {
			cdCopy.Status.LastPromotedSpec = snap.hash
		}
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

// persistSpecTracking records the (lastAppliedSpec, lastTrackedRevision) pair
// after a hash drift absorption, fence refresh, manual rollback or spec
// tracking migration decision, and rewrites lastPromotedSpec in lockstep when
// it pointed at the same spec as lastAppliedSpec (idle canary). The write is
// based on the status values the decision was computed from and is aborted if
// another writer changed them in the meantime; cd is never mutated in place.
func persistSpecTracking(flaggerClient clientset.Interface, cd *flaggerv1.Canary, snap targetSnapshot) error {
	name, ns := cd.GetName(), cd.GetNamespace()
	basedOnApplied := cd.Status.LastAppliedSpec
	basedOnPromoted := cd.Status.LastPromotedSpec
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh, err := flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
		}
		if fresh.Status.LastAppliedSpec != basedOnApplied {
			// another writer updated the tracked spec; the next reconcile
			// will re-make the decision against the fresh status
			return nil
		}
		if fresh.Status.LastAppliedSpec == snap.hash && fresh.Status.LastTrackedRevision == snap.fence {
			// already persisted
			return nil
		}

		cdCopy := fresh.DeepCopy()
		cdCopy.Status.LastAppliedSpec = snap.hash
		cdCopy.Status.LastTrackedRevision = snap.fence
		if basedOnPromoted != "" && basedOnPromoted == basedOnApplied {
			cdCopy.Status.LastPromotedSpec = snap.hash
		}

		return updateStatusWithUpgrade(flaggerClient, cdCopy)
	})
	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

// refreshTrackedRevision updates lastTrackedRevision after Flagger's own
// writes to the target (scaling), which advance the server change counter
// without altering the normalized spec. The fence only moves while the
// recorded lastAppliedSpec still matches the hash of the written object: a
// mismatch means a concurrent change is pending (or the status predates the
// tracking format) and the stale fence must remain in place so the change is
// detected by hash comparison instead of being absorbed.
func refreshTrackedRevision(flaggerClient clientset.Interface, cd *flaggerv1.Canary, snap targetSnapshot) error {
	name, ns := cd.GetName(), cd.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh, err := flaggerClient.FlaggerV1beta1().Canaries(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
		}
		if fresh.Status.LastAppliedSpec != snap.hash || fresh.Status.LastTrackedRevision == snap.fence {
			return nil
		}

		cdCopy := fresh.DeepCopy()
		cdCopy.Status.LastTrackedRevision = snap.fence

		return updateStatusWithUpgrade(flaggerClient, cdCopy)
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

		if phase != flaggerv1.CanaryPhaseProgressing && phase != flaggerv1.CanaryPhaseWaiting && phase != flaggerv1.CanaryPhasePromoting {
			cdCopy.Status.CanaryWeight = 0
			cdCopy.Status.Iterations = 0
			if phase == flaggerv1.CanaryPhaseWaitingPromotion {
				cdCopy.Status.Iterations = cd.GetAnalysis().Iterations - 1
			}
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
	case flaggerv1.CanaryPhaseWaitingPromotion:
		status = corev1.ConditionUnknown
		message = "Waiting for approval."
	case flaggerv1.CanaryPhaseProgressing:
		status = corev1.ConditionUnknown
		message = "New revision detected, progressing canary analysis."
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
