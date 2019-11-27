package router

import (
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// KubernetesServiceRouter manages ClusterIP services
type KubernetesServiceRouter struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	labelSelector string
	annotations   map[string]string
	ports         map[string]int32
}

// Reconcile creates or updates the primary and canary services to prepare for the canary release process targeted on the K8s service
func (c *KubernetesServiceRouter) Reconcile(canary *flaggerv1.Canary) error {
	targetName := canary.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	svc, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// canary svc
	err = c.reconcileCanaryService(canary, canaryName, svc)
	if err != nil {
		return err
	}

	// primary svc
	err = c.reconcilePrimaryService(canary, primaryName, svc)
	if err != nil {
		return err
	}

	return nil
}

func (c *KubernetesServiceRouter) SetRoutes(canary *flaggerv1.Canary, primaryRoute int, canaryRoute int) error {
	return nil
}

func (c *KubernetesServiceRouter) GetRoutes(canary *flaggerv1.Canary) (primaryRoute int, canaryRoute int, err error) {
	return 0, 0, nil
}

func (c *KubernetesServiceRouter) reconcileCanaryService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
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

func (c *KubernetesServiceRouter) reconcilePrimaryService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
	_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return c.createService(canary, name, src)
	}

	if err != nil {
		return fmt.Errorf("service %s query error %v", name, err)
	}

	return nil
}

func (c *KubernetesServiceRouter) createService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
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
