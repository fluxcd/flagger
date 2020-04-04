package router

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

type IngressRouter struct {
	kubeClient        kubernetes.Interface
	annotationsPrefix string
	logger            *zap.SugaredLogger
}

func (i *IngressRouter) Reconcile(canary *flaggerv1.Canary) error {
	if canary.Spec.IngressRef == nil || canary.Spec.IngressRef.Name == "" {
		return fmt.Errorf("ingress selector is empty")
	}

	apexName, _, _ := canary.GetServiceNames()
	canaryName := fmt.Sprintf("%s-canary", apexName)
	canaryIngressName := fmt.Sprintf("%s-canary", canary.Spec.IngressRef.Name)

	ingress, err := i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(context.TODO(), canary.Spec.IngressRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("ingress %s.%s get query error: %w", canary.Spec.IngressRef.Name, canary.Namespace, err)
	}

	ingressClone := ingress.DeepCopy()

	// change backend to <deployment-name>-canary
	backendExists := false
	for k, v := range ingressClone.Spec.Rules {
		for x, y := range v.HTTP.Paths {
			if y.Backend.ServiceName == apexName {
				ingressClone.Spec.Rules[k].HTTP.Paths[x].Backend.ServiceName = canaryName
				backendExists = true
				break
			}
		}
	}

	if !backendExists {
		return fmt.Errorf("backend %s not found in ingress %s", apexName, canary.Spec.IngressRef.Name)
	}

	canaryIngress, err := i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		ing := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      canaryIngressName,
				Namespace: canary.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
				Annotations: i.makeAnnotations(ingressClone.Annotations),
				Labels:      ingressClone.Labels,
			},
			Spec: ingressClone.Spec,
		}

		_, err := i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Create(context.TODO(), ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("ingress %s.%s create error: %w", ing.Name, ing.Namespace, err)
		}

		i.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Ingress %s.%s created", ing.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("ingress %s.%s query error: %w", canaryIngressName, canary.Namespace, err)
	}

	if diff := cmp.Diff(ingressClone.Spec, canaryIngress.Spec); diff != "" {
		iClone := canaryIngress.DeepCopy()
		iClone.Spec = ingressClone.Spec

		_, err := i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Update(context.TODO(), iClone, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("ingress %s.%s update error: %w", canaryIngressName, iClone.Namespace, err)
		}

		i.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Ingress %s updated", canaryIngressName)
	}

	return nil
}

func (i *IngressRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	canaryIngressName := fmt.Sprintf("%s-canary", canary.Spec.IngressRef.Name)
	canaryIngress, err := i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("ingress %s.%s get query error: %w", canaryIngressName, canary.Namespace, err)
		return
	}

	// A/B testing
	if len(canary.GetAnalysis().Match) > 0 {
		for k := range canaryIngress.Annotations {
			if k == i.GetAnnotationWithPrefix("canary-by-cookie") || k == i.GetAnnotationWithPrefix("canary-by-header") {
				return 0, 100, false, nil
			}
		}
	}

	// Canary
	for k, v := range canaryIngress.Annotations {
		if k == i.GetAnnotationWithPrefix("canary-weight") {
			val, errAtoi := strconv.Atoi(v)
			if errAtoi != nil {
				err = fmt.Errorf("failed to convert %s to int: %w", v, errAtoi)
				return
			}

			canaryWeight = val
			break
		}
	}

	primaryWeight = 100 - canaryWeight
	mirrored = false
	return
}

func (i *IngressRouter) SetRoutes(
	canary *flaggerv1.Canary,
	_ int,
	canaryWeight int,
	_ bool,
) error {
	canaryIngressName := fmt.Sprintf("%s-canary", canary.Spec.IngressRef.Name)
	canaryIngress, err := i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("ingress %s.%s get query error: %w", canaryIngressName, canary.Namespace, err)
	}

	iClone := canaryIngress.DeepCopy()

	// A/B testing
	if len(canary.GetAnalysis().Match) > 0 {
		var cookie, header, headerValue, headerRegex string
		for _, m := range canary.GetAnalysis().Match {
			for k, v := range m.Headers {
				if k == "cookie" {
					cookie = v.Exact
				} else {
					header = k
					headerRegex = v.Regex
					headerValue = v.Exact
				}
			}
		}

		iClone.Annotations = i.makeHeaderAnnotations(iClone.Annotations, header, headerValue, headerRegex, cookie)
	} else {
		// canary
		iClone.Annotations[i.GetAnnotationWithPrefix("canary-weight")] = fmt.Sprintf("%v", canaryWeight)
	}

	// toggle canary
	if canaryWeight > 0 {
		iClone.Annotations[i.GetAnnotationWithPrefix("canary")] = "true"
	} else {
		iClone.Annotations = i.makeAnnotations(iClone.Annotations)
	}

	_, err = i.kubeClient.NetworkingV1beta1().Ingresses(canary.Namespace).Update(context.TODO(), iClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("ingress %s.%s update error %v", iClone.Name, iClone.Namespace, err)
	}

	return nil
}

func (i *IngressRouter) makeAnnotations(annotations map[string]string) map[string]string {
	res := make(map[string]string)
	for k, v := range annotations {
		if !strings.Contains(k, i.GetAnnotationWithPrefix("canary")) &&
			!strings.Contains(k, "kubectl.kubernetes.io/last-applied-configuration") {
			res[k] = v
		}
	}

	res[i.GetAnnotationWithPrefix("canary")] = "false"

	return res
}

func (i *IngressRouter) makeHeaderAnnotations(annotations map[string]string,
	header string, headerValue string, headerRegex string, cookie string) map[string]string {
	res := make(map[string]string)
	for k, v := range annotations {
		if !strings.Contains(v, i.GetAnnotationWithPrefix("canary")) {
			res[k] = v
		}
	}

	res[i.GetAnnotationWithPrefix("canary")] = "true"

	if cookie != "" {
		res[i.GetAnnotationWithPrefix("canary-by-cookie")] = cookie
	}

	if header != "" {
		res[i.GetAnnotationWithPrefix("canary-by-header")] = header
	}

	if headerValue != "" {
		res[i.GetAnnotationWithPrefix("canary-by-header-value")] = headerValue
	}

	if headerRegex != "" {
		res[i.GetAnnotationWithPrefix("canary-by-header-pattern")] = headerRegex
	}

	return res
}

func (i *IngressRouter) GetAnnotationWithPrefix(suffix string) string {
	return fmt.Sprintf("%v/%v", i.annotationsPrefix, suffix)
}

func (i *IngressRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
