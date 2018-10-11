package controller

import (
	"fmt"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

type CanaryRouter struct {
	kubeClient    kubernetes.Interface
	istioClient   istioclientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Sync creates the primary and canary ClusterIP services
// and sets up a virtual service with routes for the two services
// all traffic goes to primary
func (c *CanaryRouter) Sync(cd *flaggerv1.Canary) error {
	err := c.createServices(cd)
	if err != nil {
		return err
	}
	err = c.createVirtualService(cd)
	if err != nil {
		return err
	}
	return nil
}

func (c *CanaryRouter) createServices(cd *flaggerv1.Canary) error {
	canaryName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", canaryName)
	canaryService, err := c.kubeClient.CoreV1().Services(cd.Namespace).Get(canaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		canaryService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      canaryName,
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
				Selector: map[string]string{"app": canaryName},
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
		c.logger.Infof("Service %s.%s created", canaryService.GetName(), cd.Namespace)
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
				Selector: map[string]string{"app": canaryName},
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
		c.logger.Infof("Service %s.%s created", canaryTestService.GetName(), cd.Namespace)
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

		c.logger.Infof("Service %s.%s created", primaryService.GetName(), cd.Namespace)
	}

	return nil
}

func (c *CanaryRouter) createVirtualService(cd *flaggerv1.Canary) error {
	canaryName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", canaryName)
	hosts := append(cd.Spec.Service.Hosts, canaryName)
	gateways := append(cd.Spec.Service.Gateways, "mesh")
	virtualService, err := c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(canaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		virtualService = &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cd.Name,
				Namespace: cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: istiov1alpha3.VirtualServiceSpec{
				Hosts:    hosts,
				Gateways: gateways,
				Http: []istiov1alpha3.HTTPRoute{
					{
						Route: []istiov1alpha3.DestinationWeight{
							{
								Destination: istiov1alpha3.Destination{
									Host: primaryName,
									Port: istiov1alpha3.PortSelector{
										Number: uint32(cd.Spec.Service.Port),
									},
								},
								Weight: 100,
							},
							{
								Destination: istiov1alpha3.Destination{
									Host: canaryName,
									Port: istiov1alpha3.PortSelector{
										Number: uint32(cd.Spec.Service.Port),
									},
								},
								Weight: 0,
							},
						},
					},
				},
			},
		}

		_, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Create(virtualService)
		if err != nil {
			return fmt.Errorf("VirtualService %s.%s create error %v", cd.Name, cd.Namespace, err)
		}
		c.logger.Infof("VirtualService %s.%s created", virtualService.GetName(), cd.Namespace)
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (c *CanaryRouter) GetRoutes(cd *flaggerv1.Canary) (
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
	err error,
) {
	vs := &istiov1alpha3.VirtualService{}
	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(cd.Name, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			err = fmt.Errorf("VirtualService %s.%s not found", cd.Name, cd.Namespace)
			return
		}
		err = fmt.Errorf("VirtualService %s.%s query error %v", cd.Name, cd.Namespace, err)
		return
	}

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name) {
				primary = route
			}
			if route.Destination.Host == cd.Spec.TargetRef.Name {
				canary = route
			}
		}
	}

	if primary.Weight == 0 && canary.Weight == 0 {
		err = fmt.Errorf("VirtualService %s.%s does not contain routes for %s and %s",
			cd.Name, cd.Namespace, fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name), cd.Spec.TargetRef.Name)
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (c *CanaryRouter) SetRoutes(
	cd *flaggerv1.Canary,
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
) error {
	vs, err := c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(cd.Name, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("VirtualService %s.%s not found", cd.Name, cd.Namespace)

		}
		return fmt.Errorf("VirtualService %s.%s query error %v", cd.Name, cd.Namespace, err)
	}
	vs.Spec.Http = []istiov1alpha3.HTTPRoute{
		{
			Route: []istiov1alpha3.DestinationWeight{primary, canary},
		},
	}

	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Update(vs)
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s update failed: %v", cd.Name, cd.Namespace, err)

	}
	return nil
}
