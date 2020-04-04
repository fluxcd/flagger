package router

import (
	"context"
	"encoding/json"
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

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// KubernetesDefaultRouter is managing ClusterIP services
type KubernetesDefaultRouter struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	labelSelector string
	ports         map[string]int32
}

// Initialize creates the primary and canary services
func (c *KubernetesDefaultRouter) Initialize(canary *flaggerv1.Canary) error {
	_, primaryName, canaryName := canary.GetServiceNames()

	// canary svc
	err := c.reconcileService(canary, canaryName, canary.Spec.TargetRef.Name, canary.Spec.Service.Canary)
	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}

	// primary svc
	err = c.reconcileService(canary, primaryName, fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name), canary.Spec.Service.Primary)
	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}

	return nil
}

// Reconcile creates or updates the main service
func (c *KubernetesDefaultRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, _, _ := canary.GetServiceNames()

	// main svc
	err := c.reconcileService(canary, apexName, fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name), canary.Spec.Service.Apex)
	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}

	return nil
}

func (c *KubernetesDefaultRouter) SetRoutes(_ *flaggerv1.Canary, _ int, _ int) error {
	return nil
}

func (c *KubernetesDefaultRouter) GetRoutes(_ *flaggerv1.Canary) (primaryRoute int, canaryRoute int, err error) {
	return 0, 0, nil
}

func (c *KubernetesDefaultRouter) reconcileService(canary *flaggerv1.Canary, name string, podSelector string, metadata *flaggerv1.CustomMetadata) error {
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

	// set pod selector and apex port
	svcSpec := corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: map[string]string{c.labelSelector: podSelector},
		Ports: []corev1.ServicePort{
			{
				Name:       portName,
				Protocol:   corev1.ProtocolTCP,
				Port:       canary.Spec.Service.Port,
				TargetPort: targetPort,
			},
		},
	}

	// set additional ports
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

	if metadata == nil {
		metadata = &flaggerv1.CustomMetadata{}
	}

	if metadata.Labels == nil {
		metadata.Labels = make(map[string]string)
	}
	metadata.Labels[c.labelSelector] = name

	if metadata.Annotations == nil {
		metadata.Annotations = make(map[string]string)
	}

	c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Debugw(fmt.Sprintf("Creating Service %s", name), "metadata", metadata, "service_configuration", canary.Spec.Service)

	// create service if it doesn't exists
	svc, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   canary.Namespace,
				Labels:      metadata.Labels,
				Annotations: metadata.Annotations,
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

		_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("service %s.%s create error: %w", svc.Name, canary.Namespace, err)
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Service %s.%s created", svc.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("service %s get query error: %w", name, err)
	}

	// update existing service pod selector and ports
	if svc != nil {
		sortPorts := func(a, b interface{}) bool {
			return a.(corev1.ServicePort).Port < b.(corev1.ServicePort).Port
		}

		// copy node ports from existing service
		for _, port := range svc.Spec.Ports {
			for i, servicePort := range svcSpec.Ports {
				if port.Name == servicePort.Name && port.NodePort > 0 {
					svcSpec.Ports[i].NodePort = port.NodePort
					break
				}
			}
		}

		updateService := false
		svcClone := svc.DeepCopy()

		portsDiff := cmp.Diff(svcSpec.Ports, svc.Spec.Ports, cmpopts.SortSlices(sortPorts))
		selectorsDiff := cmp.Diff(svcSpec.Selector, svc.Spec.Selector)

		if portsDiff != "" || selectorsDiff != "" {
			svcClone.Spec.Ports = svcSpec.Ports
			svcClone.Spec.Selector = svcSpec.Selector
			_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Update(context.TODO(), svcClone, metav1.UpdateOptions{})
			updateService = true
		}

		// update annotations and labels only if the service has been created by Flagger
		if _, owned := c.isOwnedByCanary(svc, canary.Name); owned {
			if cmp.Diff(metadata.Annotations, svc.ObjectMeta.Annotations) != "" {
				svcClone.ObjectMeta.Annotations = metadata.Annotations
				updateService = true
			}
			if cmp.Diff(metadata.Labels, svc.ObjectMeta.Labels) != "" {
				svcClone.ObjectMeta.Labels = metadata.Labels
				updateService = true
			}
		}

		if updateService {
			_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Update(context.TODO(), svcClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("service %s update error: %w", name, err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Service %s updated", svc.GetName())
		}
	}

	return nil
}

// Finalize reverts the apex router if not owned by the Flagger controller.
func (c *KubernetesDefaultRouter) Finalize(canary *flaggerv1.Canary) error {
	apexName, _, _ := canary.GetServiceNames()

	svc, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	// No need to do any reconciliation if the router is owned by the controller
	if hasCanaryOwnerRef, isOwned := c.isOwnedByCanary(svc, canary.Name); !hasCanaryOwnerRef && !isOwned {
		// If kubectl annotation is present that will be utilized, else reconcile
		if a, ok := svc.Annotations[kubectlAnnotation]; ok {
			var storedSvc corev1.Service
			if err := json.Unmarshal([]byte(a), &storedSvc); err != nil {
				return fmt.Errorf("router %s.%s failed to unMarshal annotation %s",
					svc.Name, svc.Namespace, kubectlAnnotation)
			}
			clone := svc.DeepCopy()
			clone.Spec.Selector = storedSvc.Spec.Selector

			if _, err := c.kubeClient.CoreV1().Services(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("service %s update error: %w", clone.Name, err)
			}
		} else {
			err = c.reconcileService(canary, apexName, canary.Spec.TargetRef.Name, nil)
			if err != nil {
				return fmt.Errorf("reconcileService failed: %w", err)
			}
		}
	}
	return nil
}

// isOwnedByCanary evaluates if an object contains an OwnerReference declaration, that is of kind Canary and
// has the same ref name as the Canary under evaluation.  It returns two bool the first returns true if
// an OwnerReference is present and the second returns true if it is owned by the supplied name.
func (c KubernetesDefaultRouter) isOwnedByCanary(obj interface{}, name string) (bool, bool) {
	object, ok := obj.(metav1.Object)
	if !ok {
		return false, false
	}

	ownerRef := metav1.GetControllerOf(object)
	if ownerRef == nil {
		return false, false
	}

	if ownerRef.Kind != flaggerv1.CanaryKind {
		return false, false
	}

	return true, ownerRef.Name == name
}
