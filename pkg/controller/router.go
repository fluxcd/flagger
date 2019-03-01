package controller

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
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

// CanaryRouter is managing the operations for Kubernetes service kind
// and Istio virtual services
type CanaryRouter struct {
	kubeClient    kubernetes.Interface
	istioClient   istioclientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Sync creates or updates the primary and canary ClusterIP services
// and the Istio virtual service.
func (c *CanaryRouter) Sync(cd *flaggerv1.Canary) error {
	err := c.createServices(cd)
	if err != nil {
		return err
	}
	err = c.syncVirtualService(cd)
	if err != nil {
		return err
	}
	return nil
}

func (c *CanaryRouter) createServices(cd *flaggerv1.Canary) error {
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

func (c *CanaryRouter) syncVirtualService(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	hosts := append(cd.Spec.Service.Hosts, targetName)
	gateways := cd.Spec.Service.Gateways
	var hasMeshGateway bool
	for _, g := range gateways {
		if g == "mesh" {
			hasMeshGateway = true
		}
	}
	if !hasMeshGateway {
		gateways = append(gateways, "mesh")
	}

	route := []istiov1alpha3.DestinationWeight{
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
				Host: fmt.Sprintf("%s-canary", targetName),
				Port: istiov1alpha3.PortSelector{
					Number: uint32(cd.Spec.Service.Port),
				},
			},
			Weight: 0,
		},
	}
	newSpec := istiov1alpha3.VirtualServiceSpec{
		Hosts:    hosts,
		Gateways: gateways,
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match:   cd.Spec.Service.Match,
				Rewrite: cd.Spec.Service.Rewrite,
				Timeout: cd.Spec.Service.Timeout,
				Retries: cd.Spec.Service.Retries,
				Route:   route,
			},
		},
	}

	virtualService, err := c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(targetName, metav1.GetOptions{})
	// insert
	if errors.IsNotFound(err) {
		virtualService = &istiov1alpha3.VirtualService{
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
			Spec: newSpec,
		}
		_, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Create(virtualService)
		if err != nil {
			return fmt.Errorf("VirtualService %s.%s create error %v", targetName, cd.Namespace, err)
		}
		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
			Infof("VirtualService %s.%s created", virtualService.GetName(), cd.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("VirtualService %s.%s query error %v", targetName, cd.Namespace, err)
	}

	// update service but keep the original destination weights
	if virtualService != nil {
		if diff := cmp.Diff(newSpec, virtualService.Spec, cmpopts.IgnoreTypes(istiov1alpha3.DestinationWeight{})); diff != "" {
			//fmt.Println(diff)
			vtClone := virtualService.DeepCopy()
			vtClone.Spec = newSpec
			if len(virtualService.Spec.Http) > 0 {
				vtClone.Spec.Http[0].Route = virtualService.Spec.Http[0].Route
			}
			_, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Update(vtClone)
			if err != nil {
				return fmt.Errorf("VirtualService %s.%s update error %v", targetName, cd.Namespace, err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("VirtualService %s.%s updated", virtualService.GetName(), cd.Namespace)
		}
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (c *CanaryRouter) GetRoutes(cd *flaggerv1.Canary) (
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
	err error,
) {
	targetName := cd.Spec.TargetRef.Name
	vs := &istiov1alpha3.VirtualService{}
	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(targetName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			err = fmt.Errorf("VirtualService %s.%s not found", targetName, cd.Namespace)
			return
		}
		err = fmt.Errorf("VirtualService %s.%s query error %v", targetName, cd.Namespace, err)
		return
	}

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == fmt.Sprintf("%s-primary", targetName) {
				primary = route
			}
			if route.Destination.Host == fmt.Sprintf("%s-canary", targetName) {
				canary = route
			}
		}
	}

	if primary.Weight == 0 && canary.Weight == 0 {
		err = fmt.Errorf("VirtualService %s.%s does not contain routes for %s-primary and %s-canary",
			targetName, cd.Namespace, targetName, targetName)
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (c *CanaryRouter) SetRoutes(
	cd *flaggerv1.Canary,
	primary istiov1alpha3.DestinationWeight,
	canary istiov1alpha3.DestinationWeight,
) error {
	targetName := cd.Spec.TargetRef.Name
	vs, err := c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(targetName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("VirtualService %s.%s not found", targetName, cd.Namespace)

		}
		return fmt.Errorf("VirtualService %s.%s query error %v", targetName, cd.Namespace, err)
	}

	vsCopy := vs.DeepCopy()
	vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
		{
			Match:   cd.Spec.Service.Match,
			Rewrite: cd.Spec.Service.Rewrite,
			Timeout: cd.Spec.Service.Timeout,
			Retries: cd.Spec.Service.Retries,
			Route:   []istiov1alpha3.DestinationWeight{primary, canary},
		},
	}

	vs, err = c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Update(vsCopy)
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s update failed: %v", targetName, cd.Namespace, err)

	}
	return nil
}
