package router

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

/*
Skipper Principles:

* if only one backend has a weight, only one backend will get 100% traffic
* if two of three backends has a weight, only two should get traffic.
* if two backends doesn't have any weight, they get equal amount of traffic.
* weights can be int or float, but always relative.

Implementation:
* apex Ingress is immutable
* new canary Ingress contains two paths for primary and canary service
* canary Ingress manages weights on primary & canary service, hence no traffic to apex service

*/

const (
	skipperBackendWeightsAnnotationKey = "zalando.org/backend-weights"
	canaryPatternf                     = "%s-canary"
)

type SkipperRouter struct {
	kubeClient kubernetes.Interface
	logger     *zap.SugaredLogger
}

// Reconcile creates or updates the ingresses
func (skp *SkipperRouter) Reconcile(canary *flaggerv1.Canary) error {
	if canary.Spec.IngressRef == nil || canary.Spec.IngressRef.Name == "" {
		return fmt.Errorf("ingress selector is empty")
	}

	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()
	apexIngressName, canaryIngressName := skp.getIngressNames(canary.Spec.IngressRef.Name)

	// retrieving apex ingress
	apexIngress, err := skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(
		context.TODO(), apexIngressName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("apexIngress %s.%s get query error: %w", apexIngressName, canary.Namespace, err)
	}

	// building the canary ingress from apex
	iClone := apexIngress.DeepCopy()
	for x := range iClone.Spec.Rules {
		rule := &iClone.Spec.Rules[x] // ref not value
		for y := range rule.HTTP.Paths {
			path := &rule.HTTP.Paths[y] // ref not value
			if path.Backend.ServiceName == apexSvcName {
				// flipping to primary service
				path.Backend.ServiceName = primarySvcName
				// adding second canary service
				canaryBackend := path.DeepCopy()
				canaryBackend.Backend.ServiceName = canarySvcName
				rule.HTTP.Paths = append(rule.HTTP.Paths, *canaryBackend)
			}
		}
	}
	if apexIngress.DeepCopy() == iClone {
		return fmt.Errorf("backend %s not found in ingress %s", apexSvcName, apexIngressName)
	}

	iClone.Annotations = skp.makeAnnotations(iClone.Annotations, map[string]int{primarySvcName: 100, canarySvcName: 0})
	iClone.Name = canaryIngressName
	iClone.Namespace = canary.Namespace
	iClone.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(canary, schema.GroupVersionKind{
			Group:   flaggerv1.SchemeGroupVersion.Group,
			Version: flaggerv1.SchemeGroupVersion.Version,
			Kind:    flaggerv1.CanaryKind,
		}),
	}

	// search for existence
	canaryIngress, err := skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(
		context.TODO(), canaryIngressName, metav1.GetOptions{})

	// new ingress
	if errors.IsNotFound(err) {
		_, err := skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Create(context.TODO(), iClone, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("ingress %s.%s create error: %w", iClone.Name, iClone.Namespace, err)
		}
		skp.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Ingress %s.%s created", iClone.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("ingress %s.%s query error: %w", canaryIngressName, canary.Namespace, err)
	}

	// existant, updating
	diffSpec := cmp.Diff(iClone.Spec, canaryIngress.Spec)
	diffAnnotations := cmp.Diff(iClone.Annotations, canaryIngress.Annotations)
	if diffSpec != "" || diffAnnotations != "" {
		ingressClone := canaryIngress.DeepCopy()
		ingressClone.Spec = iClone.Spec
		ingressClone.Annotations = iClone.Annotations

		_, err := skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Update(context.TODO(), ingressClone, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("ingress %s.%s update error: %w", canaryIngressName, ingressClone.Namespace, err)
		}
		skp.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Ingress %s updated", canaryIngressName)
	}

	return nil
}

func (skp *SkipperRouter) GetRoutes(canary *flaggerv1.Canary) (primaryWeight, canaryWeight int, mirrored bool, err error) {
	_, primarySvcName, canarySvcName := canary.GetServiceNames()

	_, canaryIngressName := skp.getIngressNames(canary.Spec.IngressRef.Name)
	canaryIngress, err := skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("ingress %s.%s get query error: %w", canaryIngressName, canary.Namespace, err)
		return
	}

	weights, err := skp.backendWeights(canaryIngress.Annotations)
	if err != nil {
		err = fmt.Errorf("ingress %s.%s get backendWeights error: %w", canaryIngressName, canary.Namespace, err)
		return
	}
	primaryWeight = weights[primarySvcName]
	canaryWeight = weights[canarySvcName]
	mirrored = false
	return
}

func (skp *SkipperRouter) SetRoutes(canary *flaggerv1.Canary, primaryWeight, canaryWeight int, _ bool) (err error) {
	_, primarySvcName, canarySvcName := canary.GetServiceNames()
	_, canaryIngressName := skp.getIngressNames(canary.Spec.IngressRef.Name)
	canaryIngress, err := skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("ingress %s.%s get query error: %w", canaryIngressName, canary.Namespace, err)
	}

	iClone := canaryIngress.DeepCopy()

	// TODO: A/B testing

	// Canary
	iClone.Annotations = skp.makeAnnotations(iClone.Annotations, map[string]int{
		primarySvcName: primaryWeight,
		canarySvcName:  canaryWeight,
	})

	_, err = skp.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Update(context.TODO(), iClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("ingress %s.%s update error %v", iClone.Name, iClone.Namespace, err)
	}

	return nil
}

func (skp *SkipperRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}

func (skp *SkipperRouter) makeAnnotations(annotations map[string]string, backendWeights map[string]int) map[string]string {
	b, err := json.Marshal(backendWeights)
	if err != nil {
		skp.logger.Errorf("Skipper:makeAnnotations: unable to marshal backendWeights %w", err)
		return annotations
	}
	annotations[skipperBackendWeightsAnnotationKey] = string(b)
	return annotations
}

// parse backend-weights annotation if it exists
func (skp *SkipperRouter) backendWeights(annotation map[string]string) (backendWeights map[string]int, err error) {
	backends, ok := annotation[skipperBackendWeightsAnnotationKey]
	if ok {
		err = json.Unmarshal([]byte(backends), &backendWeights)
	} else {
		err = errors.NewNotFound(schema.GroupResource{Group: "Skipper Canary Ingress", Resource: "Annotation"},
			skipperBackendWeightsAnnotationKey)
	}
	return
}

// getIngressNames returns the primary and canary Kubernetes Ingress names
func (skp *SkipperRouter) getIngressNames(name string) (apexName, canaryName string) {
	return name, fmt.Sprintf(canaryPatternf, name)
}
