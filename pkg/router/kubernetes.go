package router

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// KubernetesRouter is managing ClusterIP services
type KubernetesRouter struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	label         string
	ports         *map[string]int32
}

// Reconcile creates or updates the primary and canary services
func (c *KubernetesRouter) Reconcile(canary *flaggerv1.Canary) error {
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

func (c *KubernetesRouter) SetRoutes(canary *flaggerv1.Canary, primaryRoute int, canaryRoute int) error {
	return nil
}

func (c *KubernetesRouter) GetRoutes(canary *flaggerv1.Canary) (primaryRoute int, canaryRoute int, err error) {
	return 0, 0, nil
}

func (c *KubernetesRouter) reconcileService(canary *flaggerv1.Canary, name string, target string) error {
	portName := canary.Spec.Service.PortName
	if portName == "" {
		portName = "http"
	}

	svcSpec := corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: map[string]string{c.label: target},
		Ports: []corev1.ServicePort{
			{
				Name:     portName,
				Protocol: corev1.ProtocolTCP,
				Port:     canary.Spec.Service.Port,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: canary.Spec.Service.Port,
				},
			},
		},
	}

	if c.ports != nil {
		for n, p := range *c.ports {
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
	}

	svc, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: canary.Namespace,
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
		portsDiff := cmp.Diff(svcSpec.Ports, svc.Spec.Ports)
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
