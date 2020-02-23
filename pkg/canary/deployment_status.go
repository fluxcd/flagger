package canary

import (
	"fmt"

	ex "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// SyncStatus encodes the canary pod spec and updates the canary status
func (c *DeploymentController) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	dep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return ex.Wrap(err, "SyncStatus deployment query error")
	}

	configs, err := c.configTracker.GetConfigRefs(cd)
	if err != nil {
		return ex.Wrap(err, "SyncStatus configs query error")
	}

	return syncCanaryStatus(c.flaggerClient, cd, status, dep.Spec.Template, func(cdCopy *flaggerv1.Canary) {
		cdCopy.Status.TrackedConfigs = configs
	})
}

// SetStatusFailedChecks updates the canary failed checks counter
func (c *DeploymentController) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	return setStatusFailedChecks(c.flaggerClient, cd, val)
}

// SetStatusWeight updates the canary status weight value
func (c *DeploymentController) SetStatusWeight(cd *flaggerv1.Canary, val int) error {
	return setStatusWeight(c.flaggerClient, cd, val)
}

// SetStatusIterations updates the canary status iterations value
func (c *DeploymentController) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	return setStatusIterations(c.flaggerClient, cd, val)
}

// SetStatusPhase updates the canary status phase
func (c *DeploymentController) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	return setStatusPhase(c.flaggerClient, cd, phase)
}
