package controller

import (
	"fmt"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Controller) bootstrapDeployment(cd *flaggerv1.Canary) error {

	canaryName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(canaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found, retrying in %v",
				canaryName, cd.Namespace, c.rolloutWindow)
		} else {
			return err
		}
	}

	primaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		primaryDep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        primaryName,
				Annotations: canaryDep.Annotations,
				Namespace:   cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: canaryDep.Spec.Replicas,
				Strategy: canaryDep.Spec.Strategy,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": primaryName,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app": primaryName},
						Annotations: canaryDep.Spec.Template.Annotations,
					},
					Spec: canaryDep.Spec.Template.Spec,
				},
			},
		}

		_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Create(primaryDep)
		if err != nil {
			return err
		}

		c.recordEventInfof(cd, "Deployment %s.%s created", primaryDep.GetName(), cd.Namespace)
	}

	if cd.Status.State == "" {
		c.scaleToZeroCanary(cd)
	}

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
		c.recordEventInfof(cd, "Service %s.%s created", canaryService.GetName(), cd.Namespace)
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
		c.recordEventInfof(cd, "Service %s.%s created", canaryTestService.GetName(), cd.Namespace)
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

		c.recordEventInfof(cd, "Service %s.%s created", primaryService.GetName(), cd.Namespace)
	}

	hosts := append(cd.Spec.Service.Hosts, canaryName)
	gateways := append(cd.Spec.Service.Gateways, "mesh")
	virtualService, err := c.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(canaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		virtualService = &istiov1alpha3.VirtualService{
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
			return err
		}
		c.recordEventInfof(cd, "VirtualService %s.%s created", virtualService.GetName(), cd.Namespace)
	}

	return nil
}
