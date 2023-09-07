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
	"strings"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1beta1"
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
	v1beta1PathMatchType    = v1beta1.PathMatchPathPrefix
	v1beta1PathMatchRegex   = v1beta1.PathMatchRegularExpression
	v1beta1PathMatchExact   = v1beta1.PathMatchExact
	v1beta1PathMatchPrefix  = v1beta1.PathMatchPathPrefix
	v1beta1HeaderMatchExact = v1beta1.HeaderMatchExact
	v1beta1HeaderMatchRegex = v1beta1.HeaderMatchRegularExpression
	v1beta1QueryMatchExact  = v1beta1.QueryParamMatchExact
	v1beta1QueryMatchRegex  = v1beta1.QueryParamMatchRegularExpression
)

type GatewayAPIV1Beta1Router struct {
	gatewayAPIClient clientset.Interface
	kubeClient       kubernetes.Interface
	logger           *zap.SugaredLogger
	setOwnerRefs     bool
}

func (gwr *GatewayAPIV1Beta1Router) Reconcile(canary *flaggerv1.Canary) error {
	if len(canary.Spec.Service.GatewayRefs) == 0 {
		return fmt.Errorf("GatewayRefs must be specified when using Gateway API as a provider.")
	}

	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()

	hrNamespace := canary.Namespace

	var hostNames []v1beta1.Hostname
	for _, host := range canary.Spec.Service.Hosts {
		hostNames = append(hostNames, v1beta1.Hostname(host))
	}
	matches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
	if err != nil {
		return fmt.Errorf("Invalid request matching selectors: %w", err)
	}
	if len(matches) == 0 {
		matches = append(matches, v1beta1.HTTPRouteMatch{
			Path: &v1beta1.HTTPPathMatch{
				Type:  &v1beta1PathMatchType,
				Value: &pathMatchValue,
			},
		})
	}

	httpRouteSpec := v1beta1.HTTPRouteSpec{
		CommonRouteSpec: v1beta1.CommonRouteSpec{
			ParentRefs: canary.Spec.Service.GatewayRefs,
		},
		Hostnames: hostNames,
		Rules: []v1beta1.HTTPRouteRule{
			{
				Matches: matches,
				BackendRefs: []v1beta1.HTTPBackendRef{
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
		httpRouteSpec.Rules[0].Matches = gwr.mergeMatchConditions(analysisMatches, matches)
		httpRouteSpec.Rules = append(httpRouteSpec.Rules, v1beta1.HTTPRouteRule{
			Matches: matches,
			BackendRefs: []v1beta1.HTTPBackendRef{
				{
					BackendRef: gwr.makeBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port),
				},
			},
		})
	}

	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes(hrNamespace).Get(
		context.TODO(), apexSvcName, metav1.GetOptions{},
	)

	newMetadata := canary.Spec.Service.Apex
	if newMetadata == nil {
		newMetadata = &flaggerv1.CustomMetadata{}
	}
	if newMetadata.Labels == nil {
		newMetadata.Labels = make(map[string]string)
	}
	if newMetadata.Annotations == nil {
		newMetadata.Annotations = make(map[string]string)
	}
	newMetadata.Annotations = filterMetadata(newMetadata.Annotations)

	if errors.IsNotFound(err) {
		route := &v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:        apexSvcName,
				Namespace:   hrNamespace,
				Labels:      newMetadata.Labels,
				Annotations: newMetadata.Annotations,
			},
			Spec: httpRouteSpec,
		}

		if gwr.setOwnerRefs {
			route.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
		}

		_, err := gwr.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes(hrNamespace).
			Create(context.TODO(), route, metav1.CreateOptions{})

		if err != nil {
			return fmt.Errorf("HTTPRoute %s.%s create error: %w", apexSvcName, hrNamespace, err)
		}
		gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("HTTPRoute %s.%s created", route.GetName(), hrNamespace)
	} else if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
	}

	ignoreCmpOptions := []cmp.Option{
		cmpopts.IgnoreFields(v1beta1.BackendRef{}, "Weight"),
		cmpopts.EquateEmpty(),
	}
	if canary.Spec.Analysis.SessionAffinity != nil {
		ignoreRoute := cmpopts.IgnoreSliceElements(func(r v1beta1.HTTPRouteRule) bool {
			// Ignore the rule that does sticky routing, i.e. matches against the `Cookie` header.
			for _, match := range r.Matches {
				for _, headerMatch := range match.Headers {
					if *headerMatch.Type == v1beta1HeaderMatchRegex && headerMatch.Name == cookieHeader &&
						strings.Contains(headerMatch.Value, canary.Spec.Analysis.SessionAffinity.CookieName) {
						return true
					}
				}
			}
			return false
		})
		ignoreCmpOptions = append(ignoreCmpOptions, ignoreRoute)
		// Ignore backend specific filters, since we use that to insert the `Set-Cookie` header in responses.
		ignoreCmpOptions = append(ignoreCmpOptions, cmpopts.IgnoreFields(v1beta1.HTTPBackendRef{}, "Filters"))
	}

	if httpRoute != nil {
		specDiff := cmp.Diff(
			httpRoute.Spec, httpRouteSpec,
			ignoreCmpOptions...,
		)
		labelsDiff := cmp.Diff(newMetadata.Labels, httpRoute.Labels, cmpopts.EquateEmpty())
		annotationsDiff := cmp.Diff(newMetadata.Annotations, httpRoute.Annotations, cmpopts.EquateEmpty())
		if (specDiff != "" && httpRoute.Name != "") || labelsDiff != "" || annotationsDiff != "" {
			hrClone := httpRoute.DeepCopy()
			hrClone.Spec = httpRouteSpec
			hrClone.ObjectMeta.Annotations = newMetadata.Annotations
			hrClone.ObjectMeta.Labels = newMetadata.Labels
			_, err := gwr.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes(hrNamespace).
				Update(context.TODO(), hrClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("HTTPRoute %s.%s update error: %w while reconciling", hrClone.GetName(), hrNamespace, err)
			}
			gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("HTTPRoute %s.%s updated", hrClone.GetName(), hrNamespace)
		}
	}

	return nil
}

func (gwr *GatewayAPIV1Beta1Router) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()
	hrNamespace := canary.Namespace
	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes(hrNamespace).Get(context.TODO(), apexSvcName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
		return
	}
	var weightedRule *v1beta1.HTTPRouteRule
	for _, rule := range httpRoute.Spec.Rules {
		// If session affinity is enabled, then we are only interested in the rule
		// that has backend-specific filters, as that's the rule that does weighted
		// routing.
		if canary.Spec.Analysis.SessionAffinity != nil {
			for _, backendRef := range rule.BackendRefs {
				if len(backendRef.Filters) > 0 {
					weightedRule = &rule
				}
			}
		}

		// A/B testing: Avoid reading the rule with only for backendRef.
		if len(rule.BackendRefs) == 2 {
			for _, backendRef := range rule.BackendRefs {
				if backendRef.Name == v1beta1.ObjectName(primarySvcName) {
					primaryWeight = int(*backendRef.Weight)
				}
				if backendRef.Name == v1beta1.ObjectName(canarySvcName) {
					canaryWeight = int(*backendRef.Weight)
				}
			}
		}
	}

	if weightedRule != nil {
		for _, backendRef := range weightedRule.BackendRefs {
			if backendRef.Name == v1beta1.ObjectName(primarySvcName) {
				primaryWeight = int(*backendRef.Weight)
			}
			if backendRef.Name == v1beta1.ObjectName(canarySvcName) {
				canaryWeight = int(*backendRef.Weight)
			}
		}
	}
	return
}

func (gwr *GatewayAPIV1Beta1Router) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
) error {
	pWeight := int32(primaryWeight)
	cWeight := int32(canaryWeight)
	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()
	hrNamespace := canary.Namespace
	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes(hrNamespace).Get(context.TODO(), apexSvcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
	}
	hrClone := httpRoute.DeepCopy()
	hostNames := []v1beta1.Hostname{}
	for _, host := range canary.Spec.Service.Hosts {
		hostNames = append(hostNames, v1beta1.Hostname(host))
	}
	matches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
	if err != nil {
		return fmt.Errorf("Invalid request matching selectors: %w", err)
	}
	if len(matches) == 0 {
		matches = append(matches, v1beta1.HTTPRouteMatch{
			Path: &v1beta1.HTTPPathMatch{
				Type:  &v1beta1PathMatchType,
				Value: &pathMatchValue,
			},
		})
	}
	weightedRouteRule := &v1beta1.HTTPRouteRule{
		Matches: matches,
		BackendRefs: []v1beta1.HTTPBackendRef{
			{
				BackendRef: gwr.makeBackendRef(primarySvcName, pWeight, canary.Spec.Service.Port),
			},
			{
				BackendRef: gwr.makeBackendRef(canarySvcName, cWeight, canary.Spec.Service.Port),
			},
		},
	}
	httpRouteSpec := v1beta1.HTTPRouteSpec{
		CommonRouteSpec: v1beta1.CommonRouteSpec{
			ParentRefs: canary.Spec.Service.GatewayRefs,
		},
		Hostnames: hostNames,
		Rules: []v1beta1.HTTPRouteRule{
			*weightedRouteRule,
		},
	}

	if canary.Spec.Analysis.SessionAffinity != nil {
		rules, err := gwr.getSessionAffinityRouteRules(canary, canaryWeight, weightedRouteRule)
		if err != nil {
			return err
		}
		httpRouteSpec.Rules = rules
	}

	hrClone.Spec = httpRouteSpec

	// A/B testing
	if len(canary.GetAnalysis().Match) > 0 {
		analysisMatches, _ := gwr.mapRouteMatches(canary.GetAnalysis().Match)
		hrClone.Spec.Rules[0].Matches = gwr.mergeMatchConditions(analysisMatches, matches)
		hrClone.Spec.Rules = append(hrClone.Spec.Rules, v1beta1.HTTPRouteRule{
			Matches: matches,
			BackendRefs: []v1beta1.HTTPBackendRef{
				{
					BackendRef: gwr.makeBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port),
				},
			},
		})
	}

	_, err = gwr.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes(hrNamespace).Update(context.TODO(), hrClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s update error: %w while setting weights", hrClone.GetName(), hrNamespace, err)
	}

	return nil
}

func (gwr *GatewayAPIV1Beta1Router) Finalize(_ *flaggerv1.Canary) error {
	return nil
}

// getSessionAffinityRouteRules returns the HTTPRouteRule objects required to perform
// session affinity based Canary releases.
func (gwr *GatewayAPIV1Beta1Router) getSessionAffinityRouteRules(canary *flaggerv1.Canary, canaryWeight int,
	weightedRouteRule *v1beta1.HTTPRouteRule) ([]v1beta1.HTTPRouteRule, error) {
	_, primarySvcName, canarySvcName := canary.GetServiceNames()
	stickyRouteRule := *weightedRouteRule

	// If a canary run is active, we want all responses corresponding to requests hitting the canary deployment
	// (due to weighted routing) to include a `Set-Cookie` header. All requests that have the `Cookie` header
	// and match the value of the `Set-Cookie` header will be routed to the canary deployment.
	if canaryWeight != 0 {
		if canary.Status.SessionAffinityCookie == "" {
			canary.Status.SessionAffinityCookie = fmt.Sprintf("%s=%s", canary.Spec.Analysis.SessionAffinity.CookieName, randSeq())
		}

		// Add `Set-Cookie` header modifier to the primary backend in the weighted routing rule.
		for i, backendRef := range weightedRouteRule.BackendRefs {
			if string(backendRef.BackendObjectReference.Name) == canarySvcName {
				backendRef.Filters = append(backendRef.Filters, v1beta1.HTTPRouteFilter{
					Type: v1beta1.HTTPRouteFilterResponseHeaderModifier,
					ResponseHeaderModifier: &v1beta1.HTTPHeaderFilter{
						Add: []v1beta1.HTTPHeader{
							{
								Name: setCookieHeader,
								Value: fmt.Sprintf("%s; %s=%d", canary.Status.SessionAffinityCookie, maxAgeAttr,
									canary.Spec.Analysis.SessionAffinity.GetMaxAge(),
								),
							},
						},
					},
				})
			}
			weightedRouteRule.BackendRefs[i] = backendRef
		}

		// Add `Cookie` header matcher to the sticky routing rule.
		cookieKeyAndVal := strings.Split(canary.Status.SessionAffinityCookie, "=")
		regexMatchType := v1beta1.HeaderMatchRegularExpression
		cookieMatch := v1beta1.HTTPRouteMatch{
			Headers: []v1beta1.HTTPHeaderMatch{
				{
					Type:  &regexMatchType,
					Name:  cookieHeader,
					Value: fmt.Sprintf(".*%s.*%s.*", cookieKeyAndVal[0], cookieKeyAndVal[1]),
				},
			},
		}

		svcMatches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
		if err != nil {
			return nil, err
		}

		mergedMatches := gwr.mergeMatchConditions([]v1beta1.HTTPRouteMatch{cookieMatch}, svcMatches)
		stickyRouteRule.Matches = mergedMatches
		stickyRouteRule.BackendRefs = []v1beta1.HTTPBackendRef{
			{
				BackendRef: gwr.makeBackendRef(primarySvcName, 0, canary.Spec.Service.Port),
			},
			{
				BackendRef: gwr.makeBackendRef(canarySvcName, 100, canary.Spec.Service.Port),
			},
		}
	} else {
		// If canary weight is 0 and SessionAffinityCookie is non-blank, then it belongs to a previous canary run.
		if canary.Status.SessionAffinityCookie != "" {
			canary.Status.PreviousSessionAffinityCookie = canary.Status.SessionAffinityCookie
		}
		previousCookie := canary.Status.PreviousSessionAffinityCookie

		// Match against the previous session cookie and delete that cookie
		if previousCookie != "" {
			cookieKeyAndVal := strings.Split(previousCookie, "=")
			regexMatchType := v1beta1.HeaderMatchRegularExpression
			cookieMatch := v1beta1.HTTPRouteMatch{
				Headers: []v1beta1.HTTPHeaderMatch{
					{
						Type:  &regexMatchType,
						Name:  cookieHeader,
						Value: fmt.Sprintf(".*%s.*%s.*", cookieKeyAndVal[0], cookieKeyAndVal[1]),
					},
				},
			}
			svcMatches, _ := gwr.mapRouteMatches(canary.Spec.Service.Match)
			mergedMatches := gwr.mergeMatchConditions([]v1beta1.HTTPRouteMatch{cookieMatch}, svcMatches)
			stickyRouteRule.Matches = mergedMatches

			stickyRouteRule.Filters = append(stickyRouteRule.Filters, v1beta1.HTTPRouteFilter{
				Type: v1beta1.HTTPRouteFilterResponseHeaderModifier,
				ResponseHeaderModifier: &v1beta1.HTTPHeaderFilter{
					Add: []v1beta1.HTTPHeader{
						{
							Name:  setCookieHeader,
							Value: fmt.Sprintf("%s; %s=%d", previousCookie, maxAgeAttr, -1),
						},
					},
				},
			})
		}

		canary.Status.SessionAffinityCookie = ""
	}

	return []v1beta1.HTTPRouteRule{stickyRouteRule, *weightedRouteRule}, nil
}

func (gwr *GatewayAPIV1Beta1Router) mapRouteMatches(requestMatches []v1alpha3.HTTPMatchRequest) ([]v1beta1.HTTPRouteMatch, error) {
	matches := []v1beta1.HTTPRouteMatch{}

	for _, requestMatch := range requestMatches {
		match := v1beta1.HTTPRouteMatch{}
		if requestMatch.Uri != nil {
			if requestMatch.Uri.Regex != "" {
				match.Path = &v1beta1.HTTPPathMatch{
					Type:  &v1beta1PathMatchRegex,
					Value: &requestMatch.Uri.Regex,
				}
			} else if requestMatch.Uri.Exact != "" {
				match.Path = &v1beta1.HTTPPathMatch{
					Type:  &v1beta1PathMatchExact,
					Value: &requestMatch.Uri.Exact,
				}
			} else if requestMatch.Uri.Prefix != "" {
				match.Path = &v1beta1.HTTPPathMatch{
					Type:  &v1beta1PathMatchPrefix,
					Value: &requestMatch.Uri.Prefix,
				}
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified path matching selector: %+v\n", requestMatch.Uri)
			}
		}
		if requestMatch.Method != nil {
			if requestMatch.Method.Exact != "" {
				method := v1beta1.HTTPMethod(requestMatch.Method.Exact)
				match.Method = &method
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified header matching selector: %+v\n", requestMatch.Headers)
			}
		}
		for key, val := range requestMatch.Headers {
			headerMatch := v1beta1.HTTPHeaderMatch{}
			if val.Exact != "" {
				headerMatch.Name = v1beta1.HTTPHeaderName(key)
				headerMatch.Type = &v1beta1HeaderMatchExact
				headerMatch.Value = val.Exact
			} else if val.Regex != "" {
				headerMatch.Name = v1beta1.HTTPHeaderName(key)
				headerMatch.Type = &v1beta1HeaderMatchRegex
				headerMatch.Value = val.Regex
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified header matching selector: %+v\n", requestMatch.Headers)
			}
			if (v1beta1.HTTPHeaderMatch{} != headerMatch) {
				match.Headers = append(match.Headers, headerMatch)
			}
		}

		for key, val := range requestMatch.QueryParams {
			queryMatch := v1beta1.HTTPQueryParamMatch{}
			if val.Exact != "" {
				queryMatch.Name = key
				queryMatch.Type = &v1beta1QueryMatchExact
				queryMatch.Value = val.Exact
			} else if val.Regex != "" {
				queryMatch.Name = key
				queryMatch.Type = &v1beta1QueryMatchRegex
				queryMatch.Value = val.Regex
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified query matching selector: %+v\n", requestMatch.QueryParams)
			}

			if (v1beta1.HTTPQueryParamMatch{} != queryMatch) {
				match.QueryParams = append(match.QueryParams, queryMatch)
			}
		}

		if !reflect.DeepEqual(match, v1beta1.HTTPRouteMatch{}) {
			matches = append(matches, match)
		}
	}

	return matches, nil
}

func (gwr *GatewayAPIV1Beta1Router) makeBackendRef(svcName string, weight, port int32) v1beta1.BackendRef {
	return v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Group: (*v1beta1.Group)(&backendRefGroup),
			Kind:  (*v1beta1.Kind)(&backendRefKind),
			Name:  v1beta1.ObjectName(svcName),
			Port:  (*v1beta1.PortNumber)(&port),
		},
		Weight: &weight,
	}
}

func (gwr *GatewayAPIV1Beta1Router) mergeMatchConditions(analysis, service []v1beta1.HTTPRouteMatch) []v1beta1.HTTPRouteMatch {
	if len(analysis) == 0 {
		return service
	}
	if len(service) == 0 {
		return analysis
	}

	merged := make([]v1beta1.HTTPRouteMatch, len(service)*len(analysis))
	num := 0
	for _, a := range analysis {
		for _, s := range service {
			merged[num] = *s.DeepCopy()
			if len(a.Headers) > 0 {
				merged[num].Headers = a.Headers
			}
			if len(a.QueryParams) > 0 {
				merged[num].QueryParams = a.QueryParams
			}
			num++
		}
	}
	return merged
}
