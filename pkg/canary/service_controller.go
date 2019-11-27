package canary

import (
	"fmt"

	ex "github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// ServiceController is managing the operations for Kubernetes service kind
type ServiceController struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// SetStatusFailedChecks updates the canary failed checks counter
func (c *ServiceController) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	return setStatusFailedChecks(c.flaggerClient, cd, val)
}

// SetStatusWeight updates the canary status weight value
func (c *ServiceController) SetStatusWeight(cd *flaggerv1.Canary, val int) error {
	return setStatusWeight(c.flaggerClient, cd, val)
}

// SetStatusIterations updates the canary status iterations value
func (c *ServiceController) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	return setStatusIterations(c.flaggerClient, cd, val)
}

// SetStatusPhase updates the canary status phase
func (c *ServiceController) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	return setStatusPhase(c.flaggerClient, cd, phase)
}

var _ Controller = &ServiceController{}

// Initialize creates the primary deployment, hpa,
// scales to zero the canary deployment and returns the pod selector label and container ports
func (c *ServiceController) Initialize(cd *flaggerv1.Canary, skipLivenessChecks bool) (label string, ports map[string]int32, err error) {
	return "", nil, nil
}

// Promote copies target's spec from canary to primary
func (c *ServiceController) Promote(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	canary, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("service %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("service %s.%s query error %v", targetName, cd.Namespace, err)
	}

	primary, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("service %s.%s not found", primaryName, cd.Namespace)
		}
		return fmt.Errorf("service %s.%s query error %v", primaryName, cd.Namespace, err)
	}

	primaryCopy := canary.DeepCopy()
	primaryCopy.ObjectMeta.Name = primary.ObjectMeta.Name
	if primaryCopy.Spec.Type == "ClusterIP" {
		primaryCopy.Spec.ClusterIP = primary.Spec.ClusterIP
	}
	primaryCopy.ObjectMeta.ResourceVersion = primary.ObjectMeta.ResourceVersion
	primaryCopy.ObjectMeta.UID = primary.ObjectMeta.UID

	// apply update
	_, err = c.kubeClient.CoreV1().Services(cd.Namespace).Update(primaryCopy)
	if err != nil {
		return fmt.Errorf("updating service %s.%s spec failed: %v",
			primaryCopy.GetName(), primaryCopy.Namespace, err)
	}

	return nil
}

// HasServiceChanged returns true if the canary service spec has changed
func (c *ServiceController) HasTargetChanged(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Errorf("service %s.%s not found", targetName, cd.Namespace)
		}
		return false, fmt.Errorf("service %s.%s query error %v", targetName, cd.Namespace, err)
	}

	return hasSpecChanged(cd, canary.Spec)
}

// Scale sets the canary deployment replicas
func (c *ServiceController) Scale(cd *flaggerv1.Canary, replicas int32) error {
	return nil
}

func (c *ServiceController) ScaleFromZero(cd *flaggerv1.Canary) error {
	return nil
}

func (c *ServiceController) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	dep, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("service %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return ex.Wrap(err, "SyncStatus service query error")
	}

	return syncCanaryStatus(c.flaggerClient, cd, status, dep.Spec, func(cdCopy *flaggerv1.Canary) {})
}

func (c *ServiceController) HaveDependenciesChanged(cd *flaggerv1.Canary) (bool, error) {
	return false, nil
}

func (c *ServiceController) IsPrimaryReady(cd *flaggerv1.Canary) (bool, error) {
	return true, nil
}

func (c *ServiceController) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	return true, nil
}
