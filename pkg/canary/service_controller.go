package canary

import (
	"context"
	"fmt"

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
func (c *ServiceController) GetMetadata(_ *flaggerv1.Canary) (string, map[string]int32, error) {
	return "", nil, nil
}

// Initialize creates or updates the primary and canary services to prepare for the canary release process targeted on the K8s service
func (c *ServiceController) Initialize(cd *flaggerv1.Canary) (err error) {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	svc, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", primaryName, cd.Namespace, err)
	}

	if err = c.reconcileCanaryService(cd, canaryName, svc); err != nil {
		return fmt.Errorf("reconcileCanaryService failed: %w", err)
	}

	if err = c.reconcilePrimaryService(cd, primaryName, svc); err != nil {
		return fmt.Errorf("reconcilePrimaryService failed: %w", err)
	}

	return nil
}

func (c *ServiceController) reconcileCanaryService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
	current, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return c.createService(canary, name, src)
	} else if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", name, canary.Namespace, err)
	}

	ns := buildService(canary, name, src)

	if ns.Spec.Type == "ClusterIP" {
		// We can't change this immutable field
		ns.Spec.ClusterIP = current.Spec.ClusterIP
	}

	// We can't change this immutable field
	ns.ObjectMeta.UID = current.ObjectMeta.UID

	ns.ObjectMeta.ResourceVersion = current.ObjectMeta.ResourceVersion

	_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Update(context.TODO(), ns, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating service %s.%s failed: %w", name, canary.Namespace, err)
	}

	c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Infof("Service %s.%s updated", ns.GetName(), canary.Namespace)
	return nil
}

func (c *ServiceController) reconcilePrimaryService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
	_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return c.createService(canary, name, src)
	} else if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", name, canary.Namespace, err)
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

	_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating service %s.%s query error: %w", canary.Name, canary.Namespace, err)
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

	canary, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	primary, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", primaryName, cd.Namespace, err)
	}

	primaryCopy := canary.DeepCopy()
	primaryCopy.ObjectMeta.Name = primary.ObjectMeta.Name
	if primaryCopy.Spec.Type == "ClusterIP" {
		primaryCopy.Spec.ClusterIP = primary.Spec.ClusterIP
	}
	primaryCopy.ObjectMeta.ResourceVersion = primary.ObjectMeta.ResourceVersion
	primaryCopy.ObjectMeta.UID = primary.ObjectMeta.UID

	// apply update
	_, err = c.kubeClient.CoreV1().Services(cd.Namespace).Update(context.TODO(), primaryCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating service %s.%s spec failed: %w",
			primaryCopy.GetName(), primaryCopy.Namespace, err)
	}

	return nil
}

// HasServiceChanged returns true if the canary service spec has changed
func (c *ServiceController) HasTargetChanged(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("service %s.%s get query error: %w", targetName, cd.Namespace, err)
	}
	return hasSpecChanged(cd, canary.Spec)
}

// Scale sets the canary deployment replicas
func (c *ServiceController) ScaleToZero(_ *flaggerv1.Canary) error {
	return nil
}

func (c *ServiceController) ScaleFromZero(_ *flaggerv1.Canary) error {
	return nil
}

func (c *ServiceController) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	dep, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	return syncCanaryStatus(c.flaggerClient, cd, status, dep.Spec, func(cdCopy *flaggerv1.Canary) {})
}

func (c *ServiceController) HaveDependenciesChanged(_ *flaggerv1.Canary) (bool, error) {
	return false, nil
}

func (c *ServiceController) IsPrimaryReady(_ *flaggerv1.Canary) error {
	return nil
}

func (c *ServiceController) IsCanaryReady(_ *flaggerv1.Canary) (bool, error) {
	return true, nil
}

func (c *ServiceController) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
