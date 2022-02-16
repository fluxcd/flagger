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
	"reflect"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1alpha2"
	"github.com/fluxcd/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

var (
	initialPrimaryWeight = int32(100)
	initialCanaryWeight  = int32(0)
	backendRefGroup      = ""
	backendRefKind       = "Service"
	pathMatchValue       = "/"
	pathMatchType        = v1alpha2.PathMatchPathPrefix
	pathMatchRegex       = v1alpha2.PathMatchRegularExpression
	pathMatchExact       = v1alpha2.PathMatchExact
	pathMatchPrefix      = v1alpha2.PathMatchPathPrefix
	headerMatchExact     = v1alpha2.HeaderMatchExact
	headerMatchRegex     = v1alpha2.HeaderMatchRegularExpression
	queryMatchExact      = v1alpha2.QueryParamMatchExact
	queryMatchRegex      = v1alpha2.QueryParamMatchRegularExpression
)

type GatewayAPIRouter struct {
	gatewayAPIClient clientset.Interface
	kubeClient       kubernetes.Interface
	logger           *zap.SugaredLogger
}

func (gwr *GatewayAPIRouter) Reconcile(canary *flaggerv1.Canary) error {
	if len(canary.Spec.Service.GatewayRefs) == 0 {
		return fmt.Errorf("GatewayRefs must be specified when using Gateway API as a provider.")
	}

	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()

	hrNamespace := canary.Namespace
	if canary.Spec.TargetRef.Namespace != "" {
		hrNamespace = canary.Spec.TargetRef.Namespace
	}

	hostNames := []v1alpha2.Hostname{}
	for _, host := range canary.Spec.Service.Hosts {
		hostNames = append(hostNames, v1alpha2.Hostname(host))
	}
	matches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
	if err != nil {
		return fmt.Errorf("Invalid request matching selectors: %w", err)
	}
	if len(matches) == 0 {
		matches = append(matches, v1alpha2.HTTPRouteMatch{
			Path: &v1alpha2.HTTPPathMatch{
				Type:  &pathMatchType,
				Value: &pathMatchValue,
			},
		})
	}

	httpRouteSpec := v1alpha2.HTTPRouteSpec{
		CommonRouteSpec: v1alpha2.CommonRouteSpec{
			ParentRefs: canary.Spec.Service.GatewayRefs,
		},
		Hostnames: hostNames,
		Rules: []v1alpha2.HTTPRouteRule{
			{
				Matches: matches,
				BackendRefs: []v1alpha2.HTTPBackendRef{
					{
						BackendRef: gwr.makeBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port),
					},
					{
						BackendRef: gwr.makeBackendRef(canarySvcName, initialCanaryWeight, canary.Spec.Service.Port),
					},
				},
			},
		},
	}

	// A/B testing
	if len(canary.GetAnalysis().Match) > 0 {
		analysisMatches, _ := gwr.mapRouteMatches(canary.GetAnalysis().Match)
		// serviceMatches, _ := gwr.mapRouteMatches(canary.Spec.Service.Match)
		httpRouteSpec.Rules[0].Matches = analysisMatches
		httpRouteSpec.Rules = append(httpRouteSpec.Rules, v1alpha2.HTTPRouteRule{
			Matches: matches,
			BackendRefs: []v1alpha2.HTTPBackendRef{
				{
					BackendRef: gwr.makeBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port),
				},
			},
		})
	}

	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes(hrNamespace).Get(
		context.TODO(), apexSvcName, metav1.GetOptions{},
	)

	if errors.IsNotFound(err) {
		metadata := canary.Spec.Service.Apex
		if metadata == nil {
			metadata = &flaggerv1.CustomMetadata{}
		}
		if metadata.Labels == nil {
			metadata.Labels = make(map[string]string)
		}
		if metadata.Annotations == nil {
			metadata.Annotations = make(map[string]string)
		}
		route := &v1alpha2.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:        apexSvcName,
				Namespace:   hrNamespace,
				Labels:      metadata.Labels,
				Annotations: filterMetadata(metadata.Annotations),
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: httpRouteSpec,
		}

		_, err := gwr.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes(hrNamespace).
			Create(context.TODO(), route, metav1.CreateOptions{})

		if err != nil {
			return fmt.Errorf("HTTPRoute %s.%s create error: %w", apexSvcName, hrNamespace, err)
		}
		gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("HTTPRoute %s.%s created", route.GetName(), hrNamespace)
	} else if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
	}

	if httpRoute != nil {
		if diff := cmp.Diff(
			httpRoute.Spec, httpRouteSpec,
			cmpopts.IgnoreFields(v1alpha2.BackendRef{}, "Weight"),
		); diff != "" {
			hrClone := httpRoute.DeepCopy()
			hrClone.Spec = httpRouteSpec
			_, err := gwr.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes(hrNamespace).
				Update(context.TODO(), hrClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("HTTPRoute %s.%s update error: %w", hrClone.GetName(), hrNamespace, err)
			}
			gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("HTTPProxy %s.%s updated", hrClone.GetName(), hrNamespace)
		}
	}

	return nil
}

func (gwr *GatewayAPIRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()
	hrNamespace := canary.Namespace
	if canary.Spec.TargetRef.Namespace != "" {
		hrNamespace = canary.Spec.TargetRef.Namespace
	}
	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes(hrNamespace).Get(context.TODO(), apexSvcName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
		return
	}
	for _, rule := range httpRoute.Spec.Rules {
		// A/B testing: Avoid reading the rule with only for backendRef.
		if len(rule.BackendRefs) == 2 {
			for _, backendRef := range rule.BackendRefs {
				if backendRef.Name == v1alpha2.ObjectName(primarySvcName) {
					primaryWeight = int(*backendRef.Weight)
				}
				if backendRef.Name == v1alpha2.ObjectName(canarySvcName) {
					canaryWeight = int(*backendRef.Weight)
				}
			}
		}

	}
	return
}

func (gwr *GatewayAPIRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
) error {
	pWeight := int32(primaryWeight)
	cWeight := int32(canaryWeight)
	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()
	hrNamespace := canary.Namespace
	if canary.Spec.TargetRef.Namespace != "" {
		hrNamespace = canary.Spec.TargetRef.Namespace
	}
	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes(hrNamespace).Get(context.TODO(), apexSvcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
	}
	hrClone := httpRoute.DeepCopy()
	hostNames := []v1alpha2.Hostname{}
	for _, host := range canary.Spec.Service.Hosts {
		hostNames = append(hostNames, v1alpha2.Hostname(host))
	}
	matches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
	if err != nil {
		return fmt.Errorf("Invalid request matching selectors: %w", err)
	}
	if len(matches) == 0 {
		matches = append(matches, v1alpha2.HTTPRouteMatch{
			Path: &v1alpha2.HTTPPathMatch{
				Type:  &pathMatchType,
				Value: &pathMatchValue,
			},
		})
	}
	httpRouteSpec := v1alpha2.HTTPRouteSpec{
		CommonRouteSpec: v1alpha2.CommonRouteSpec{
			ParentRefs: canary.Spec.Service.GatewayRefs,
		},
		Hostnames: hostNames,
		Rules: []v1alpha2.HTTPRouteRule{
			{
				Matches: matches,
				BackendRefs: []v1alpha2.HTTPBackendRef{
					{
						BackendRef: gwr.makeBackendRef(primarySvcName, pWeight, canary.Spec.Service.Port),
					},
					{
						BackendRef: gwr.makeBackendRef(canarySvcName, cWeight, canary.Spec.Service.Port),
					},
				},
			},
		},
	}
	hrClone.Spec = httpRouteSpec

	// A/B testing
	if len(canary.GetAnalysis().Match) > 0 {
		analysisMatches, _ := gwr.mapRouteMatches(canary.GetAnalysis().Match)
		hrClone.Spec.Rules[0].Matches = analysisMatches
		hrClone.Spec.Rules = append(hrClone.Spec.Rules, v1alpha2.HTTPRouteRule{
			Matches: matches,
			BackendRefs: []v1alpha2.HTTPBackendRef{
				{
					BackendRef: gwr.makeBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port),
				},
			},
		})
	}

	_, err = gwr.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes(hrNamespace).Update(context.TODO(), hrClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s update error: %w", hrClone.GetName(), hrNamespace, err)
	}
	return nil
}

func (gwr *GatewayAPIRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}

func (gwr *GatewayAPIRouter) mapRouteMatches(requestMatches []v1alpha3.HTTPMatchRequest) ([]v1alpha2.HTTPRouteMatch, error) {
	matches := []v1alpha2.HTTPRouteMatch{}

	for _, requestMatch := range requestMatches {
		match := v1alpha2.HTTPRouteMatch{}
		if requestMatch.Uri != nil {
			if requestMatch.Uri.Regex != "" {
				match.Path = &v1alpha2.HTTPPathMatch{
					Type:  &pathMatchRegex,
					Value: &requestMatch.Uri.Regex,
				}
			} else if requestMatch.Uri.Exact != "" {
				match.Path = &v1alpha2.HTTPPathMatch{
					Type:  &pathMatchExact,
					Value: &requestMatch.Uri.Exact,
				}
			} else if requestMatch.Uri.Prefix != "" {
				match.Path = &v1alpha2.HTTPPathMatch{
					Type:  &pathMatchPrefix,
					Value: &requestMatch.Uri.Prefix,
				}
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified path matching selector: %+v\n", requestMatch.Uri)
			}
		}
		if requestMatch.Method != nil {
			if requestMatch.Method.Exact != "" {
				method := v1alpha2.HTTPMethod(requestMatch.Method.Exact)
				match.Method = &method
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified header matching selector: %+v\n", requestMatch.Headers)
			}
		}
		for key, val := range requestMatch.Headers {
			headerMatch := v1alpha2.HTTPHeaderMatch{}
			if val.Exact != "" {
				headerMatch.Name = v1alpha2.HTTPHeaderName(key)
				headerMatch.Type = &headerMatchExact
				headerMatch.Value = val.Exact
			} else if val.Regex != "" {
				headerMatch.Name = v1alpha2.HTTPHeaderName(key)
				headerMatch.Type = &headerMatchRegex
				headerMatch.Value = val.Regex
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified header matching selector: %+v\n", requestMatch.Headers)
			}
			if (v1alpha2.HTTPHeaderMatch{} != headerMatch) {
				match.Headers = append(match.Headers, headerMatch)
			}
		}

		for key, val := range requestMatch.QueryParams {
			queryMatch := v1alpha2.HTTPQueryParamMatch{}
			if val.Exact != "" {
				queryMatch.Name = key
				queryMatch.Type = &queryMatchExact
				queryMatch.Value = val.Exact
			} else if val.Regex != "" {
				queryMatch.Name = key
				queryMatch.Type = &queryMatchRegex
				queryMatch.Value = val.Regex
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified query matching selector: %+v\n", requestMatch.QueryParams)
			}

			if (v1alpha2.HTTPQueryParamMatch{} != queryMatch) {
				match.QueryParams = append(match.QueryParams, queryMatch)
			}
		}

		if !reflect.DeepEqual(match, v1alpha2.HTTPRouteMatch{}) {
			matches = append(matches, match)
		}
	}

	return matches, nil
}

func (gwr *GatewayAPIRouter) makeBackendRef(svcName string, weight, port int32) v1alpha2.BackendRef {
	return v1alpha2.BackendRef{
		BackendObjectReference: v1alpha2.BackendObjectReference{
			Group: (*v1alpha2.Group)(&backendRefGroup),
			Kind:  (*v1alpha2.Kind)(&backendRefKind),
			Name:  v1alpha2.ObjectName(svcName),
			Port:  (*v1alpha2.PortNumber)(&port),
		},
		Weight: &weight,
	}
}
