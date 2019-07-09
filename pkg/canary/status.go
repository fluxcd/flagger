package canary

import (
	"fmt"

	"github.com/mitchellh/hashstructure"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SyncStatus encodes the canary pod spec and updates the canary status
func (c *Deployer) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	dep, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	configs, err := c.ConfigTracker.GetConfigRefs(cd)
	if err != nil {
		return fmt.Errorf("configs query error %v", err)
	}

	hash, err := hashstructure.Hash(dep.Spec.Template, nil)
	if err != nil {
		return fmt.Errorf("hash error %v", err)
	}

	cdCopy := cd.DeepCopy()
	cdCopy.Status.Phase = status.Phase
	cdCopy.Status.CanaryWeight = status.CanaryWeight
	cdCopy.Status.FailedChecks = status.FailedChecks
	cdCopy.Status.Iterations = status.Iterations
	cdCopy.Status.LastAppliedSpec = fmt.Sprintf("%d", hash)
	cdCopy.Status.LastTransitionTime = metav1.Now()
	cdCopy.Status.TrackedConfigs = configs

	if ok, conditions := c.makeStatusConditions(cd.Status, status.Phase); ok {
		cdCopy.Status.Conditions = conditions
	}

	cd, err = c.FlaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).UpdateStatus(cdCopy)
	if err != nil {
		return fmt.Errorf("canary %s.%s status update error %v", cdCopy.Name, cdCopy.Namespace, err)
	}
	return nil
}

// SetStatusFailedChecks updates the canary failed checks counter
func (c *Deployer) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	cdCopy := cd.DeepCopy()
	cdCopy.Status.FailedChecks = val
	cdCopy.Status.LastTransitionTime = metav1.Now()

	cd, err := c.FlaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).UpdateStatus(cdCopy)
	if err != nil {
		return fmt.Errorf("canary %s.%s status update error %v", cdCopy.Name, cdCopy.Namespace, err)
	}
	return nil
}

// SetStatusWeight updates the canary status weight value
func (c *Deployer) SetStatusWeight(cd *flaggerv1.Canary, val int) error {
	cdCopy := cd.DeepCopy()
	cdCopy.Status.CanaryWeight = val
	cdCopy.Status.LastTransitionTime = metav1.Now()

	cd, err := c.FlaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).UpdateStatus(cdCopy)
	if err != nil {
		return fmt.Errorf("canary %s.%s status update error %v", cdCopy.Name, cdCopy.Namespace, err)
	}
	return nil
}

// SetStatusIterations updates the canary status iterations value
func (c *Deployer) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	cdCopy := cd.DeepCopy()
	cdCopy.Status.Iterations = val
	cdCopy.Status.LastTransitionTime = metav1.Now()

	cd, err := c.FlaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).UpdateStatus(cdCopy)
	if err != nil {
		return fmt.Errorf("canary %s.%s status update error %v", cdCopy.Name, cdCopy.Namespace, err)
	}
	return nil
}

// SetStatusPhase updates the canary status phase
func (c *Deployer) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	cdCopy := cd.DeepCopy()
	cdCopy.Status.Phase = phase
	cdCopy.Status.LastTransitionTime = metav1.Now()

	if phase != flaggerv1.CanaryPhaseProgressing {
		cdCopy.Status.CanaryWeight = 0
		cdCopy.Status.Iterations = 0
	}

	if ok, conditions := c.makeStatusConditions(cdCopy.Status, phase); ok {
		cdCopy.Status.Conditions = conditions
	}

	cd, err := c.FlaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).UpdateStatus(cdCopy)
	if err != nil {
		return fmt.Errorf("canary %s.%s status update error %v", cdCopy.Name, cdCopy.Namespace, err)
	}
	return nil
}

// GetStatusCondition returns a condition based on type
func (c *Deployer) getStatusCondition(status flaggerv1.CanaryStatus, conditionType flaggerv1.CanaryConditionType) *flaggerv1.CanaryCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}

// MakeStatusCondition updates the canary status conditions based on canary phase
func (c *Deployer) makeStatusConditions(canaryStatus flaggerv1.CanaryStatus,
	phase flaggerv1.CanaryPhase) (bool, []flaggerv1.CanaryCondition) {
	currentCondition := c.getStatusCondition(canaryStatus, flaggerv1.PromotedType)

	message := "New deployment detected, starting initialization."
	status := corev1.ConditionUnknown
	switch phase {
	case flaggerv1.CanaryPhaseProgressing:
		status = corev1.ConditionUnknown
		message = "New revision detected, starting canary analysis."
	case flaggerv1.CanaryPhaseInitialized:
		status = corev1.ConditionTrue
		message = "New deployment detected, initialization completed."
	case flaggerv1.CanaryPhaseSucceeded:
		status = corev1.ConditionTrue
		message = "Canary analysis completed successfully, promotion finished."
	case flaggerv1.CanaryPhaseFailed:
		status = corev1.ConditionFalse
		message = "Canary analysis failed, deployment scaled to zero."
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
