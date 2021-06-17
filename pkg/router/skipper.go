/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

/*
Skipper Principles:
* if only one backend has a weight, only one backend will get 100% traffic
* if two of three or more backends have a weight, only those two should get traffic.
* if two backends don't have any weight, it's undefined and right now they get equal amount of traffic.
* weights can be int or float, but always treated as a ratio.

Implementation:
* apex Ingress is immutable
* new canary Ingress contains two paths for primary and canary service
* canary Ingress manages weights on primary & canary service, hence no traffic to apex service

*/

const (
	skipperpredicateAnnotationKey      = "zalando.org/skipper-predicate"
	skipperBackendWeightsAnnotationKey = "zalando.org/backend-weights"
	canaryPatternf                     = "%s-canary"
	canaryRouteWeight                  = "Weight(100)"
	canaryRouteDisable                 = "False()"
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
	apexIngress, err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Get(
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
			if path.Backend.Service != nil && path.Backend.Service.Name == apexSvcName {
				// flipping to primary service
				path.Backend.Service.Name = primarySvcName
				// adding second canary service
				canaryBackend := path.DeepCopy()
				canaryBackend.Backend.Service.Name = canarySvcName
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
	canaryIngress, err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Get(
		context.TODO(), canaryIngressName, metav1.GetOptions{})

	// new ingress
	if errors.IsNotFound(err) {
		// Let K8s set this. Otherwise K8s API complains with "resourceVersion should not be set on objects to be created"
		iClone.ObjectMeta.ResourceVersion = ""
		_, err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Create(context.TODO(), iClone, metav1.CreateOptions{})
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
	if cmp.Diff(iClone.Spec, canaryIngress.Spec) != "" {
		ingressClone := canaryIngress.DeepCopy()
		ingressClone.Spec = iClone.Spec
		ingressClone.Annotations = filterMetadata(iClone.Annotations)

		_, err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Update(context.TODO(), ingressClone, metav1.UpdateOptions{})
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
	canaryIngress, err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("ingress %s.%s get query error: %w", canaryIngressName, canary.Namespace, err)
		return
	}

	weights, err := skp.backendWeights(canaryIngress.Annotations)
	if err != nil {
		err = fmt.Errorf("ingress %s.%s get backendWeights error: %w", canaryIngressName, canary.Namespace, err)
		return
	}
	var ok bool
	primaryWeight, ok = weights[primarySvcName]
	if !ok {
		err = fmt.Errorf("ingress %s.%s could not get weights[primarySvcName]", canaryIngressName, canary.Namespace)
		return
	}
	canaryWeight, ok = weights[canarySvcName]
	if !ok {
		err = fmt.Errorf("ingress %s.%s could not get weights[canarySvcName]", canaryIngressName, canary.Namespace)
		return
	}
	mirrored = false
	skp.logger.With("GetRoutes", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Debugf("GetRoutes primaryWeight: %d, canaryWeight: %d", primaryWeight, canaryWeight)
	return
}

func (skp *SkipperRouter) SetRoutes(canary *flaggerv1.Canary, primaryWeight, canaryWeight int, _ bool) (err error) {
	_, primarySvcName, canarySvcName := canary.GetServiceNames()
	_, canaryIngressName := skp.getIngressNames(canary.Spec.IngressRef.Name)
	canaryIngress, err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Get(context.TODO(), canaryIngressName, metav1.GetOptions{})
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

	// Disable the canary-ingress route after the canary process
	if canaryWeight == 0 {
		// ensuring False() is at first place
		iClone.Annotations[skipperpredicateAnnotationKey] = insertPredicate(iClone.Annotations[skipperpredicateAnnotationKey], canaryRouteDisable)
	}

	_, err = skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Update(
		context.TODO(), iClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("ingress %s.%s update error %w", iClone.Name, iClone.Namespace, err)
	}
	skp.logger.With("SetRoutes", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Debugf("primaryWeight: %d, canaryWeight: %d", primaryWeight, canaryWeight)

	return err
}

func (skp *SkipperRouter) Finalize(canary *flaggerv1.Canary) error {
	gracePeriodSeconds := int64(2)
	_, canaryIngressName := skp.getIngressNames(canary.Spec.IngressRef.Name)
	skp.logger.With("deleteCanaryIngress", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Debugf("Deleting Canary Ingress: %s", canaryIngressName)

	err := skp.kubeClient.NetworkingV1().Ingresses(canary.Namespace).Delete(
		context.TODO(), canaryIngressName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriodSeconds})
	if err != nil {
		return fmt.Errorf("ingress %s.%s unable to remove canary ingress: %w", canaryIngressName, canary.Namespace, err)
	}
	return nil
}

func (skp *SkipperRouter) makeAnnotations(annotations map[string]string, backendWeights map[string]int) map[string]string {
	b, err := json.Marshal(backendWeights)
	if err != nil {
		skp.logger.Errorf("Skipper:makeAnnotations: unable to marshal backendWeights %w", err)
		return annotations
	}
	annotations[skipperBackendWeightsAnnotationKey] = string(b)
	// adding more weight to canary route solves traffic bypassing through apexIngress
	annotations[skipperpredicateAnnotationKey] = insertPredicate(annotations[skipperpredicateAnnotationKey], canaryRouteWeight)

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

func insertPredicate(raw, insert string) string {
	// ensuring it at first place
	predicates := []string{insert}
	for _, x := range strings.Split(raw, "&&") {
		predicate := strings.TrimSpace(x)
		// dropping conflicting predicates
		if predicate == "" ||
			predicate == canaryRouteWeight ||
			predicate == canaryRouteDisable {
			continue
		}
		predicates = append(predicates, predicate)
	}
	return strings.Join(predicates, " && ")
}
