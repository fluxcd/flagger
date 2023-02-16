/*
Copyright 2022 The Flux authors

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
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	a6v2 "github.com/fluxcd/flagger/pkg/apis/apisix/v2"
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ApisixRouter is managing Apisix Route
type ApisixRouter struct {
	apisixClient clientset.Interface
	logger       *zap.SugaredLogger
	setOwnerRefs bool
}

const maxPriority = 10000

// Reconcile creates or updates the Apisix Route
func (ar *ApisixRouter) Reconcile(canary *flaggerv1.Canary) error {
	if canary.Spec.RouteRef == nil || canary.Spec.RouteRef.Name == "" {
		return fmt.Errorf("apisix route selector is empty")
	}

	apisixRoute, err := ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Get(context.TODO(), canary.Spec.RouteRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("APISIX route %s.%s get query error: %w",
			canary.Spec.RouteRef.Name, canary.Namespace, err)
	}

	apisixRouteClone := apisixRoute.DeepCopy()
	if len(apisixRouteClone.Spec.HTTP) == 0 {
		return fmt.Errorf("APISIX route %s.%s's spec.http is empty",
			canary.Spec.RouteRef.Name, canary.Namespace)
	}

	apexName, primaryName, canaryName := canary.GetServiceNames()
	targetHttpRoute, _, err := ar.getTargetHttpRoute(canary, apisixRouteClone, apexName)
	if err != nil {
		return err
	}
	if len(targetHttpRoute.Backends) != 1 {
		return fmt.Errorf("APISIX route %s.%s's http route %s only one http backend is supported",
			canary.Spec.RouteRef.Name, canary.Namespace, targetHttpRoute.Name)
	}

	targetHttpRoute.Priority = maxPriority
	primaryWeight, canaryWeight := initializationWeights(canary)

	primaryBackend := targetHttpRoute.Backends[0]
	primaryBackend.ServiceName = primaryName
	primaryBackend.Weight = &primaryWeight
	targetHttpRoute.Backends[0] = primaryBackend

	canaryBackend := a6v2.ApisixRouteHTTPBackend{
		ServiceName:        canaryName,
		ServicePort:        primaryBackend.ServicePort,
		ResolveGranularity: primaryBackend.ResolveGranularity,
		Weight:             &canaryWeight,
		Subset:             primaryBackend.Subset,
	}

	targetHttpRoute.Backends = append(targetHttpRoute.Backends, canaryBackend)
	apisixRouteClone.Spec.HTTP = []a6v2.ApisixRouteHTTP{*targetHttpRoute}

	canaryApisixRouteName := fmt.Sprintf("%s-%s-canary", canary.Spec.RouteRef.Name, apexName)
	canaryApisixRoute, err := ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Get(context.TODO(), canaryApisixRouteName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		route := &a6v2.ApisixRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:        canaryApisixRouteName,
				Namespace:   canary.Namespace,
				Annotations: apisixRouteClone.Annotations,
				Labels:      apisixRouteClone.Labels,
			},
			Spec: apisixRouteClone.Spec,
		}

		if ar.setOwnerRefs {
			route.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
		}

		_, err := ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Create(context.TODO(), route, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("APISIX route %s.%s create error: %w", route.Name, route.Namespace, err)
		}

		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("APISIX route %s.%s created", route.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("APISIX route %s.%s query error: %w", canaryApisixRouteName, canary.Namespace, err)
	}

	if diff := cmp.Diff(canaryApisixRoute.Spec, apisixRouteClone.Spec,
		cmpopts.IgnoreFields(a6v2.ApisixRouteHTTPBackend{}, "Weight")); diff != "" {
		iClone := canaryApisixRoute.DeepCopy()
		iClone.Spec = apisixRouteClone.Spec

		_, err := ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Update(context.TODO(), iClone, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("APISIX route %s.%s update error: %w", canaryApisixRouteName, iClone.Namespace, err)
		}
		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Apisix route %s updated", canaryApisixRouteName)
	}

	return nil
}

func (ar *ApisixRouter) getTargetHttpRoute(canary *flaggerv1.Canary, apisixRoute *a6v2.ApisixRoute, serviceName string) (*a6v2.ApisixRouteHTTP, int, error) {
	for index, item := range apisixRoute.Spec.HTTP {
		for _, backend := range item.Backends {
			if backend.ServiceName == serviceName {
				return &item, index, nil
			}
		}
	}

	return nil, 0, fmt.Errorf("Can not find %s backend on apisix route %s.%s ",
		serviceName, canary.Spec.RouteRef.Name, canary.Namespace)
}

// GetRoutes returns the destinations weight for primary and canary
func (ar *ApisixRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, _ := canary.GetServiceNames()
	canaryApisixRouteName := fmt.Sprintf("%s-%s-canary", canary.Spec.RouteRef.Name, apexName)
	apisixRoute, err := ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Get(context.TODO(), canaryApisixRouteName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("apisix route %s.%s query error: %w", canaryApisixRouteName, canary.Namespace, err)
		return
	}
	_, targetIndex, err := ar.getTargetHttpRoute(canary, apisixRoute, primaryName)
	if err != nil {
		return
	}

	for _, backend := range apisixRoute.Spec.HTTP[targetIndex].Backends {
		if backend.ServiceName == primaryName {
			primaryWeight = *backend.Weight
			canaryWeight = 100 - primaryWeight
			return
		}
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (ar *ApisixRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	if primaryWeight == 0 && canaryWeight == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", apexName, canary.Namespace)
	}

	canaryApisixRouteName := fmt.Sprintf("%s-%s-canary", canary.Spec.RouteRef.Name, apexName)
	apisixRoute, err := ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Get(context.TODO(), canaryApisixRouteName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("apisix route %s.%s query error: %w", canaryApisixRouteName, canary.Namespace, err)
	}

	_, targetIndex, err := ar.getTargetHttpRoute(canary, apisixRoute, primaryName)
	if err != nil {
		return err
	}
	backends := apisixRoute.Spec.HTTP[targetIndex].Backends
	for i, backend := range backends {
		if backend.ServiceName == primaryName {
			backends[i].Weight = &primaryWeight
		} else if backend.ServiceName == canaryName {
			backends[i].Weight = &canaryWeight
		}
	}
	apisixRoute.Spec.HTTP[targetIndex].Backends = backends

	_, err = ar.apisixClient.ApisixV2().ApisixRoutes(canary.Namespace).Update(context.TODO(), apisixRoute, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("apisix route %s.%s update error: %w", apexName, canary.Namespace, err)
	}
	return nil
}

func (ar *ApisixRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
