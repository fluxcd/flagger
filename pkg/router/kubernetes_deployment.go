package router

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// KubernetesDeploymentRouter is managing ClusterIP services
type KubernetesDeploymentRouter struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	labelSelector string
	annotations   map[string]string
	ports         map[string]int32
}

// Reconcile creates or updates the primary and canary services
func (c *KubernetesDeploymentRouter) Reconcile(canary *flaggerv1.Canary) error {
	targetName := canary.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	// main svc
	err := c.reconcileService(canary, targetName, primaryName)
	if err != nil {
		return err
	}

	// canary svc
	err = c.reconcileService(canary, canaryName, targetName)
	if err != nil {
		return err
	}

	// primary svc
	err = c.reconcileService(canary, primaryName, primaryName)
	if err != nil {
		return err
	}

	return nil
}

func (c *KubernetesDeploymentRouter) SetRoutes(canary *flaggerv1.Canary, primaryRoute int, canaryRoute int) error {
	return nil
}

func (c *KubernetesDeploymentRouter) GetRoutes(canary *flaggerv1.Canary) (primaryRoute int, canaryRoute int, err error) {
	return 0, 0, nil
}

func (c *KubernetesDeploymentRouter) reconcileService(canary *flaggerv1.Canary, name string, target string) error {
	portName := canary.Spec.Service.PortName
	if portName == "" {
		portName = "http"
	}

	targetPort := intstr.IntOrString{
		Type:   intstr.Int,
		IntVal: canary.Spec.Service.Port,
	}

	if canary.Spec.Service.TargetPort.String() != "0" {
		targetPort = canary.Spec.Service.TargetPort
	}

	svcSpec := corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: map[string]string{c.labelSelector: target},
		Ports: []corev1.ServicePort{
			{
				Name:       portName,
				Protocol:   corev1.ProtocolTCP,
				Port:       canary.Spec.Service.Port,
				TargetPort: targetPort,
			},
		},
	}

	for n, p := range c.ports {
		cp := corev1.ServicePort{
			Name:     n,
			Protocol: corev1.ProtocolTCP,
			Port:     p,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: p,
			},
		}

		svcSpec.Ports = append(svcSpec.Ports, cp)
	}

	svc, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   canary.Namespace,
				Labels:      map[string]string{c.labelSelector: name},
				Annotations: c.annotations,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: svcSpec,
		}

		_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Create(svc)
		if err != nil {
			return err
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Service %s.%s created", svc.GetName(), canary.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("service %s query error %v", name, err)
	}

	if svc != nil {
		sortPorts := func(a, b interface{}) bool {
			return a.(corev1.ServicePort).Port < b.(corev1.ServicePort).Port
		}
		portsDiff := cmp.Diff(svcSpec.Ports, svc.Spec.Ports, cmpopts.SortSlices(sortPorts))
		selectorsDiff := cmp.Diff(svcSpec.Selector, svc.Spec.Selector)

		if portsDiff != "" || selectorsDiff != "" {
			svcClone := svc.DeepCopy()
			svcClone.Spec.Ports = svcSpec.Ports
			svcClone.Spec.Selector = svcSpec.Selector
			_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Update(svcClone)
			if err != nil {
				return fmt.Errorf("service %s update error %v", name, err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Service %s updated", svc.GetName())
		}
	}

	return nil
}

func (c *KubernetesDeploymentRouter) createService(canary *flaggerv1.Canary, name string, src *corev1.Service) error {
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
