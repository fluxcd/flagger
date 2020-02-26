package canary

import (
	"fmt"

	ex "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// SyncStatus encodes the canary pod spec and updates the canary status
func (c *DaemonSetController) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	dae, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("daemonset %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return ex.Wrap(err, "SyncStatus daemonset query error")
	}

	// ignore `daemonSetScaleDownNodeSelector` node selector
	for key := range daemonSetScaleDownNodeSelector {
		delete(dae.Spec.Template.Spec.NodeSelector, key)
	}

	// since nil and capacity zero map would have different hash, we have to initialize here
	if dae.Spec.Template.Spec.NodeSelector == nil {
		dae.Spec.Template.Spec.NodeSelector = map[string]string{}
	}

	configs, err := c.configTracker.GetConfigRefs(cd)
	if err != nil {
		return ex.Wrap(err, "SyncStatus configs query error")
	}

	return syncCanaryStatus(c.flaggerClient, cd, status, dae.Spec.Template, func(cdCopy *flaggerv1.Canary) {
		cdCopy.Status.TrackedConfigs = configs
	})
}

// SetStatusFailedChecks updates the canary failed checks counter
func (c *DaemonSetController) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	return setStatusFailedChecks(c.flaggerClient, cd, val)
}

// SetStatusWeight updates the canary status weight value
func (c *DaemonSetController) SetStatusWeight(cd *flaggerv1.Canary, val int) error {
	return setStatusWeight(c.flaggerClient, cd, val)
}

// SetStatusIterations updates the canary status iterations value
func (c *DaemonSetController) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	return setStatusIterations(c.flaggerClient, cd, val)
}

// SetStatusPhase updates the canary status phase
func (c *DaemonSetController) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	return setStatusPhase(c.flaggerClient, cd, phase)
}
