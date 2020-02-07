package canary

import (
	"fmt"

	ex "github.com/pkg/errors"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
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

// GetMetadata returns the pod label selector and svc ports
func (c *ServiceController) GetMetadata(cd *flaggerv1.Canary) (string, map[string]int32, error) {
	return "", nil, nil
}

// Initialize creates or updates the primary and canary services to prepare for the canary release process targeted on the K8s service
func (c *ServiceController) Initialize(cd *flaggerv1.Canary, skipLivenessChecks bool) (err error) {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	svc, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// canary svc
	err = c.reconcileCanaryService(cd, canaryName, svc)
	if err != nil {
		return err
	}

	// primary svc
	err = c.reconcilePrimaryService(cd, primaryName, svc)
	if err != nil {
		return err
	}

	return nil
}

func (c *ServiceController) reconcileCanaryService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
	current, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return c.createService(canary, name, src)
	}

	if err != nil {
		return fmt.Errorf("service %s query error %v", name, err)
	}

	new := buildService(canary, name, src)

	if new.Spec.Type == "ClusterIP" {
		// We can't change this immutable field
		new.Spec.ClusterIP = current.Spec.ClusterIP
	}

	// We can't change this immutable field
	new.ObjectMeta.UID = current.ObjectMeta.UID

	new.ObjectMeta.ResourceVersion = current.ObjectMeta.ResourceVersion

	_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Update(new)
	if err != nil {
		return err
	}

	c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Infof("Service %s.%s updated", new.GetName(), canary.Namespace)
	return nil
}

func (c *ServiceController) reconcilePrimaryService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
	_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return c.createService(canary, name, src)
	}

	if err != nil {
		return fmt.Errorf("service %s query error %v", name, err)
	}

	return nil
}

func (c *ServiceController) createService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
	svc := buildService(canary, name, src)

	if svc.Spec.Type == "ClusterIP" {
		// Reset and let K8s assign the IP. Otherwise we get an error due to the IP is already assigned
		svc.Spec.ClusterIP = ""
	}

	// Let K8s set this. Otherwise K8s API complains with "resourceVersion should not be set on objects to be created"
	svc.ObjectMeta.ResourceVersion = ""

	_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Create(svc)
	if err != nil {
		return err
	}

	c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Infof("Service %s.%s created", svc.GetName(), canary.Namespace)
	return nil
}

func buildService(canary *flaggerv1.Canary, name string, src *corev1.Service) *corev1.Service {
	svc := src.DeepCopy()
	svc.ObjectMeta.Name = name
	svc.ObjectMeta.Namespace = canary.Namespace
	svc.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(canary, schema.GroupVersionKind{
			Group:   flaggerv1.SchemeGroupVersion.Group,
			Version: flaggerv1.SchemeGroupVersion.Version,
			Kind:    flaggerv1.CanaryKind,
		}),
	}
	_, exists := svc.ObjectMeta.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	if exists {
		// Leaving this results in updates from flagger to this svc never succeed due to resourceVersion mismatch:
		//   Operation cannot be fulfilled on services "mysvc-canary": the object has been modified; please apply your changes to the latest version and try again
		delete(svc.ObjectMeta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	}

	return svc
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
