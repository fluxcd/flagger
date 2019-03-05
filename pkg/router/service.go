package router

import (
	"fmt"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// ServiceRouter is managing ClusterIP services
type ServiceRouter struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Sync creates to updates the primary and canary services
func (c *ServiceRouter) Sync(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryService, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		canaryService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": targetName},
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Protocol: corev1.ProtocolTCP,
						Port:     cd.Spec.Service.Port,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: cd.Spec.Service.Port,
						},
					},
				},
			},
		}

		_, err = c.kubeClient.CoreV1().Services(cd.Namespace).Create(canaryService)
		if err != nil {
			return err
		}
		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Service %s.%s created", canaryService.GetName(), cd.Namespace)
	}

	canaryTestServiceName := fmt.Sprintf("%s-canary", cd.Spec.TargetRef.Name)
	canaryTestService, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(canaryTestServiceName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		canaryTestService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      canaryTestServiceName,
				Namespace: cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": targetName},
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Protocol: corev1.ProtocolTCP,
						Port:     cd.Spec.Service.Port,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: cd.Spec.Service.Port,
						},
					},
				},
			},
		}

		_, err = c.kubeClient.CoreV1().Services(cd.Namespace).Create(canaryTestService)
		if err != nil {
			return err
		}
		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Service %s.%s created", canaryTestService.GetName(), cd.Namespace)
	}

	primaryService, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		primaryService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      primaryName,
				Namespace: cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": primaryName},
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Protocol: corev1.ProtocolTCP,
						Port:     cd.Spec.Service.Port,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: cd.Spec.Service.Port,
						},
					},
				},
			},
		}

		_, err = c.kubeClient.CoreV1().Services(cd.Namespace).Create(primaryService)
		if err != nil {
			return err
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Service %s.%s created", primaryService.GetName(), cd.Namespace)
	}

	return nil
}

func (c *ServiceRouter) SetRoutes(canary flaggerv1.Canary, primaryRoute int, canaryRoute int) error {
	return nil
}

func (c *ServiceRouter) GetRoutes(canary flaggerv1.Canary) (primaryRoute int, canaryRoute int, err error) {
	return 0, 0, nil
}
