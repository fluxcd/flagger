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
	"slices"
	"strings"
	"time"

	"crypto/sha256"
	"encoding/json"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
	"github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1beta1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

var (
	initialPrimaryWeight = int32(100)
	initialCanaryWeight  = int32(0)
	backendRefGroup      = ""
	backendRefKind       = "Service"
	pathMatchValue       = "/"
	pathMatchType        = v1.PathMatchPathPrefix
	pathMatchRegex       = v1.PathMatchRegularExpression
	pathMatchExact       = v1.PathMatchExact
	pathMatchPrefix      = v1.PathMatchPathPrefix
	headerMatchExact     = v1.HeaderMatchExact
	headerMatchRegex     = v1.HeaderMatchRegularExpression
	queryMatchExact      = v1.QueryParamMatchExact
	queryMatchRegex      = v1.QueryParamMatchRegularExpression
	annotationPrefix     = "gateway.flagger.app/"
	labelPrefix          = "gateway.flagger.app/"
	hashAnnotation       = annotationPrefix + "hash"
	ownerLabel           = labelPrefix + "owner"
)

type GatewayAPIRouter struct {
	gatewayAPIClient clientset.Interface
	kubeClient       kubernetes.Interface
	logger           *zap.SugaredLogger
	setOwnerRefs     bool
}

func (gwr *GatewayAPIRouter) Reconcile(canary *flaggerv1.Canary) error {
	if len(canary.Spec.Service.GatewayRefs) == 0 {
		return fmt.Errorf("GatewayRefs must be specified when using Gateway API as a provider.")
	}

	apexSvcName, primarySvcName, canarySvcName := canary.GetServiceNames()

	hrNamespace := canary.Namespace

	var hostNames []v1.Hostname
	for _, host := range canary.Spec.Service.Hosts {
		hostNames = append(hostNames, v1.Hostname(host))
	}
	matches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
	if err != nil {
		return fmt.Errorf("Invalid request matching selectors: %w", err)
	}
	if len(matches) == 0 {
		matches = append(matches, v1.HTTPRouteMatch{
			Path: &v1.HTTPPathMatch{
				Type:  &pathMatchType,
				Value: &pathMatchValue,
			},
		})
	}

	httpRouteSpec := v1.HTTPRouteSpec{
		CommonRouteSpec: v1.CommonRouteSpec{
			ParentRefs: toV1ParentRefs(canary.Spec.Service.GatewayRefs),
		},
		Hostnames: hostNames,
		Rules: []v1.HTTPRouteRule{
			{
				Matches: matches,
				Filters: gwr.makeFilters(canary),
				BackendRefs: []v1.HTTPBackendRef{
					gwr.makeHTTPBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port, canary.Spec.Service.PrimaryBackend),
					gwr.makeHTTPBackendRef(canarySvcName, initialCanaryWeight, canary.Spec.Service.Port, canary.Spec.Service.CanaryBackend),
				},
			},
		},
	}
	if canary.Spec.Service.Timeout != "" {
		timeout := v1.Duration(canary.Spec.Service.Timeout)
		httpRouteSpec.Rules[0].Timeouts = &v1.HTTPRouteTimeouts{
			Request: &timeout,
		}
	}

	// A/B testing
	if len(canary.GetAnalysis().Match) > 0 {
		analysisMatches, _ := gwr.mapRouteMatches(canary.GetAnalysis().Match)
		// serviceMatches, _ := gwr.mapRouteMatches(canary.Spec.Service.Match)
		httpRouteSpec.Rules[0].Matches = gwr.mergeMatchConditions(analysisMatches, matches)
		httpRouteSpec.Rules = append(httpRouteSpec.Rules, v1.HTTPRouteRule{
			Matches: matches,
			Filters: gwr.makeFilters(canary),
			BackendRefs: []v1.HTTPBackendRef{
				gwr.makeHTTPBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port, canary.Spec.Service.PrimaryBackend),
			},
		})
		if canary.Spec.Service.Timeout != "" {
			timeout := v1.Duration(canary.Spec.Service.Timeout)
			httpRouteSpec.Rules[1].Timeouts = &v1.HTTPRouteTimeouts{
				Request: &timeout,
			}
		}
	}

	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(hrNamespace).Get(
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
		route := &v1.HTTPRoute{
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

		_, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(hrNamespace).
			Create(context.TODO(), route, metav1.CreateOptions{})

		if err != nil {
			return fmt.Errorf("HTTPRoute %s.%s create error: %w", apexSvcName, hrNamespace, err)
		}
		gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("HTTPRoute %s.%s created", route.GetName(), hrNamespace)

		if err := gwr.ensureReferenceGrants(canary, httpRouteSpec, hrNamespace); err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
	}

	ignoreCmpOptions := []cmp.Option{
		cmpopts.IgnoreFields(v1.BackendRef{}, "Weight"),
		cmpopts.EquateEmpty(),
	}

	if canary.Spec.Analysis.SessionAffinity != nil {
		ignoreCookieRouteFunc := func(name string) func(r v1.HTTPRouteRule) bool {
			return func(r v1.HTTPRouteRule) bool {
				// Ignore the rule that does sticky routing, i.e. matches against the `Cookie` header.
				for _, match := range r.Matches {
					for _, headerMatch := range match.Headers {
						if *headerMatch.Type == headerMatchRegex && headerMatch.Name == cookieHeader &&
							strings.Contains(headerMatch.Value, name) {
							return true
						}
					}
				}
				return false
			}
		}
		ignoreCanaryRoute := cmpopts.IgnoreSliceElements(ignoreCookieRouteFunc(canary.Spec.Analysis.SessionAffinity.CookieName))
		ignorePrimaryRoute := cmpopts.IgnoreSliceElements(ignoreCookieRouteFunc(canary.Spec.Analysis.SessionAffinity.PrimaryCookieName))

		ignoreCmpOptions = append(ignoreCmpOptions, ignoreCanaryRoute, ignorePrimaryRoute)
		// Ignore backend specific filters, since we use that to insert the `Set-Cookie` header in responses.
		ignoreCmpOptions = append(ignoreCmpOptions, cmpopts.IgnoreFields(v1.HTTPBackendRef{}, "Filters"))
	}

	if canary.GetAnalysis().Mirror {
		// If a Canary run is in progress, the HTTPRoute rule will have an extra filter of type RequestMirror
		// which needs to be ignored so that the requests are mirrored to the canary deployment.
		inProgress := canary.Status.Phase == flaggerv1.CanaryPhaseWaiting || canary.Status.Phase == flaggerv1.CanaryPhaseProgressing ||
			canary.Status.Phase == flaggerv1.CanaryPhaseWaitingPromotion
		if inProgress {
			ignoreCmpOptions = append(ignoreCmpOptions, cmpopts.IgnoreFields(v1.HTTPRouteRule{}, "Filters"))
		}
	}

	if httpRoute != nil {
		// Preserve the existing annotations added by other controllers such as AWS Gateway API Controller.
		mergedAnnotations := newMetadata.Annotations
		for key, val := range httpRoute.Annotations {
			if _, ok := mergedAnnotations[key]; !ok {
				mergedAnnotations[key] = val
			}
		}

		// Compare the existing HTTPRoute spec and metadata with the desired state.
		// If there are differences, update the HTTPRoute object.
		specDiff := cmp.Diff(
			httpRoute.Spec, httpRouteSpec,
			ignoreCmpOptions...,
		)
		labelsDiff := cmp.Diff(newMetadata.Labels, httpRoute.Labels, cmpopts.EquateEmpty())
		annotationsDiff := cmp.Diff(mergedAnnotations, httpRoute.Annotations, cmpopts.EquateEmpty())
		if (specDiff != "" && httpRoute.Name != "") || labelsDiff != "" || annotationsDiff != "" {
			hrClone := httpRoute.DeepCopy()
			hrClone.Spec = httpRouteSpec
			hrClone.ObjectMeta.Annotations = mergedAnnotations
			hrClone.ObjectMeta.Labels = newMetadata.Labels
			_, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(hrNamespace).
				Update(context.TODO(), hrClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("HTTPRoute %s.%s update error: %w while reconciling", hrClone.GetName(), hrNamespace, err)
			}
			gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("HTTPRoute %s.%s updated", hrClone.GetName(), hrNamespace)
		}
	}

	if err := gwr.ensureReferenceGrants(canary, httpRouteSpec, hrNamespace); err != nil {
		return err
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
	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(hrNamespace).Get(context.TODO(), apexSvcName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
		return
	}
	var weightedRule *v1.HTTPRouteRule
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
				if backendRef.Name == v1.ObjectName(primarySvcName) {
					primaryWeight = int(*backendRef.Weight)
				}
				if backendRef.Name == v1.ObjectName(canarySvcName) {
					canaryWeight = int(*backendRef.Weight)
				}
			}
		}
		for _, filter := range rule.Filters {
			if filter.Type == v1.HTTPRouteFilterRequestMirror && filter.RequestMirror != nil &&
				string(filter.RequestMirror.BackendRef.Name) == canarySvcName {
				mirrored = true
			}
		}
	}

	if weightedRule != nil {
		for _, backendRef := range weightedRule.BackendRefs {
			if backendRef.Name == v1.ObjectName(primarySvcName) {
				primaryWeight = int(*backendRef.Weight)
			}
			if backendRef.Name == v1.ObjectName(canarySvcName) {
				canaryWeight = int(*backendRef.Weight)
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
	httpRoute, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(hrNamespace).Get(context.TODO(), apexSvcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s get error: %w", apexSvcName, hrNamespace, err)
	}
	hrClone := httpRoute.DeepCopy()
	hostNames := []v1.Hostname{}
	for _, host := range canary.Spec.Service.Hosts {
		hostNames = append(hostNames, v1.Hostname(host))
	}
	matches, err := gwr.mapRouteMatches(canary.Spec.Service.Match)
	if err != nil {
		return fmt.Errorf("Invalid request matching selectors: %w", err)
	}
	if len(matches) == 0 {
		matches = append(matches, v1.HTTPRouteMatch{
			Path: &v1.HTTPPathMatch{
				Type:  &pathMatchType,
				Value: &pathMatchValue,
			},
		})
	}
	var timeout v1.Duration
	if canary.Spec.Service.Timeout != "" {
		timeout = v1.Duration(canary.Spec.Service.Timeout)
	}

	weightedRouteRule := &v1.HTTPRouteRule{
		Matches: matches,
		Filters: gwr.makeFilters(canary),
		BackendRefs: []v1.HTTPBackendRef{
			gwr.makeHTTPBackendRef(primarySvcName, pWeight, canary.Spec.Service.Port, canary.Spec.Service.PrimaryBackend),
			gwr.makeHTTPBackendRef(canarySvcName, cWeight, canary.Spec.Service.Port, canary.Spec.Service.CanaryBackend),
		},
	}
	if canary.Spec.Service.Timeout != "" {
		timeout := v1.Duration(canary.Spec.Service.Timeout)
		weightedRouteRule.Timeouts = &v1.HTTPRouteTimeouts{
			Request: &timeout,
		}
	}

	// If B/G mirroring is enabled, then add a route filter which mirrors the traffic
	// to the canary service.
	if mirrored && canary.GetAnalysis().Iterations > 0 {
		weightedRouteRule.Filters = append(weightedRouteRule.Filters, v1.HTTPRouteFilter{
			Type: v1.HTTPRouteFilterRequestMirror,
			RequestMirror: &v1.HTTPRequestMirrorFilter{
				BackendRef: v1.BackendObjectReference{
					Group: (*v1.Group)(&backendRefGroup),
					Kind:  (*v1.Kind)(&backendRefKind),
					Name:  v1.ObjectName(canarySvcName),
					Port:  (*v1.PortNumber)(&canary.Spec.Service.Port),
				},
			},
		})
	}

	httpRouteSpec := v1.HTTPRouteSpec{
		CommonRouteSpec: v1.CommonRouteSpec{
			ParentRefs: toV1ParentRefs(canary.Spec.Service.GatewayRefs),
		},
		Hostnames: hostNames,
		Rules: []v1.HTTPRouteRule{
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
		hrClone.Spec.Rules = append(hrClone.Spec.Rules, v1.HTTPRouteRule{
			Matches: matches,
			Filters: gwr.makeFilters(canary),
			BackendRefs: []v1.HTTPBackendRef{
				gwr.makeHTTPBackendRef(primarySvcName, initialPrimaryWeight, canary.Spec.Service.Port, canary.Spec.Service.PrimaryBackend),
			},
			Timeouts: &v1.HTTPRouteTimeouts{
				Request: &timeout,
			},
		})

		if canary.Spec.Service.Timeout != "" {
			timeout := v1.Duration(canary.Spec.Service.Timeout)
			hrClone.Spec.Rules[1].Timeouts = &v1.HTTPRouteTimeouts{
				Request: &timeout,
			}
		}
	}

	_, err = gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(hrNamespace).Update(context.TODO(), hrClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("HTTPRoute %s.%s update error: %w while setting weights", hrClone.GetName(), hrNamespace, err)
	}

	return nil
}

func (gwr *GatewayAPIRouter) Finalize(canary *flaggerv1.Canary) error {
	// remove reference grants
	lastError := error(nil)
	ownerLabelValue := fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)
	rgs, err := gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", ownerLabel, ownerLabelValue),
	})
	if err != nil {
		return fmt.Errorf("ReferenceGrants %s get error: %w", canary.Name, err)
	}

	for _, rg := range rgs.Items {
		err := gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(rg.Namespace).Delete(context.TODO(), rg.Name, metav1.DeleteOptions{})
		if err != nil {
			// If an error occurs while deleting the ReferenceGrant, record it and continue processing without blocking other operations.
			lastError = fmt.Errorf("ReferenceGrant %s.%s delete error: %w", rg.Name, rg.Namespace, err)
		}
	}

	return lastError
}

func getBackendByServiceName(rule *v1.HTTPRouteRule, svcName string) *v1.HTTPBackendRef {
	for i, backendRef := range rule.BackendRefs {
		if string(backendRef.BackendObjectReference.Name) == svcName {

			return &rule.BackendRefs[i]
		}
	}
	return nil
}

// getSessionAffinityRouteRules returns the HTTPRouteRule objects required to perform
// session affinity based Canary releases.
func (gwr *GatewayAPIRouter) getSessionAffinityRouteRules(canary *flaggerv1.Canary, canaryWeight int,
	weightedRouteRule *v1.HTTPRouteRule) ([]v1.HTTPRouteRule, error) {
	_, primarySvcName, canarySvcName := canary.GetServiceNames()
	stickyCanaryRouteRule := *weightedRouteRule
	stickyPrimaryRouteRule := *weightedRouteRule

	// If a canary run is active, we want all responses corresponding to requests hitting the canary deployment
	// (due to weighted routing) to include a `Set-Cookie` header. All requests that have the `Cookie` header
	// and match the value of the `Set-Cookie` header will be routed to the canary deployment.
	if canaryWeight != 0 {
		// if the status doesn't have the canary cookie, then generate a new canary cookie.
		if canary.Status.SessionAffinityCookie == "" {
			canary.Status.SessionAffinityCookie = fmt.Sprintf("%s=%s", canary.Spec.Analysis.SessionAffinity.CookieName, randSeq())
		}
		primaryCookie := fmt.Sprintf("%s=%s", canary.Spec.Analysis.SessionAffinity.PrimaryCookieName, randSeq())

		// add response modifier to the canary backend ref in the rule that does weighted routing
		// to include the canary cookie.
		canaryBackendRef := getBackendByServiceName(weightedRouteRule, canarySvcName)
		canaryBackendRef.Filters = append(canaryBackendRef.Filters, v1.HTTPRouteFilter{
			Type: v1.HTTPRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &v1.HTTPHeaderFilter{
				Add: []v1.HTTPHeader{
					{
						Name:  setCookieHeader,
						Value: canary.Spec.Analysis.SessionAffinity.BuildCookie(canary.Status.SessionAffinityCookie),
					},
				},
			},
		})

		// add response modifier to the primary backend ref in the rule that does weighted routing
		// to include the primary cookie, only if a primary cookie name has been specified.
		if canary.Spec.Analysis.SessionAffinity.PrimaryCookieName != "" {
			primaryBackendRef := getBackendByServiceName(weightedRouteRule, primarySvcName)
			interval, err := time.ParseDuration(canary.Spec.Analysis.Interval)
			if err != nil {
				return nil, fmt.Errorf("failed to parse canary interval: %w", err)
			}
			primaryBackendRef.Filters = append(primaryBackendRef.Filters, v1.HTTPRouteFilter{
				Type: v1.HTTPRouteFilterResponseHeaderModifier,
				ResponseHeaderModifier: &v1.HTTPHeaderFilter{
					Add: []v1.HTTPHeader{
						{
							Name: setCookieHeader,
							Value: fmt.Sprintf("%s; %s=%d", primaryCookie, maxAgeAttr,
								int(interval.Seconds()),
							),
						},
					},
				},
			})
		}

		// configure the sticky canary rule to match against requests that match against the
		// canary cookie and send them to the canary backend.
		cookieKeyAndVal := strings.Split(canary.Status.SessionAffinityCookie, "=")
		regexMatchType := v1.HeaderMatchRegularExpression
		cookieMatch := v1.HTTPRouteMatch{
			Headers: []v1.HTTPHeaderMatch{
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

		mergedMatches := gwr.mergeMatchConditions([]v1.HTTPRouteMatch{cookieMatch}, svcMatches)
		stickyCanaryRouteRule.Matches = mergedMatches
		stickyCanaryRouteRule.BackendRefs = []v1.HTTPBackendRef{
			gwr.makeHTTPBackendRef(primarySvcName, 0, canary.Spec.Service.Port, canary.Spec.Service.PrimaryBackend),
			gwr.makeHTTPBackendRef(canarySvcName, 100, canary.Spec.Service.Port, canary.Spec.Service.CanaryBackend),
		}

		// add a sticky primary rule to match against requests that match against the
		// primary cookie and send them to the primary backend, only if a primary cookie name has
		// been specified.
		if canary.Spec.Analysis.SessionAffinity.PrimaryCookieName != "" {
			cookieKeyAndVal = strings.Split(primaryCookie, "=")
			regexMatchType = v1.HeaderMatchRegularExpression
			primaryCookieMatch := v1.HTTPRouteMatch{
				Headers: []v1.HTTPHeaderMatch{
					{
						Type:  &regexMatchType,
						Name:  cookieHeader,
						Value: fmt.Sprintf(".*%s.*%s.*", cookieKeyAndVal[0], cookieKeyAndVal[1]),
					},
				},
			}

			svcMatches, err = gwr.mapRouteMatches(canary.Spec.Service.Match)
			if err != nil {
				return nil, err
			}

			mergedMatches = gwr.mergeMatchConditions([]v1.HTTPRouteMatch{primaryCookieMatch}, svcMatches)
			stickyPrimaryRouteRule.Matches = mergedMatches
			stickyPrimaryRouteRule.BackendRefs = []v1.HTTPBackendRef{
				gwr.makeHTTPBackendRef(primarySvcName, 100, canary.Spec.Service.Port, canary.Spec.Service.PrimaryBackend),
				gwr.makeHTTPBackendRef(canarySvcName, 0, canary.Spec.Service.Port, canary.Spec.Service.CanaryBackend),
			}
			return []v1.HTTPRouteRule{stickyCanaryRouteRule, stickyPrimaryRouteRule, *weightedRouteRule}, nil
		}

		return []v1.HTTPRouteRule{stickyCanaryRouteRule, *weightedRouteRule}, nil
	} else {
		// If canary weight is 0 and SessionAffinityCookie is non-blank, then it belongs to a previous canary run.
		if canary.Status.SessionAffinityCookie != "" {
			canary.Status.PreviousSessionAffinityCookie = canary.Status.SessionAffinityCookie
		}
		previousCookie := canary.Status.PreviousSessionAffinityCookie

		// Match against the previous session cookie and delete that cookie
		if previousCookie != "" {
			cookieKeyAndVal := strings.Split(previousCookie, "=")
			regexMatchType := v1.HeaderMatchRegularExpression
			cookieMatch := v1.HTTPRouteMatch{
				Headers: []v1.HTTPHeaderMatch{
					{
						Type:  &regexMatchType,
						Name:  cookieHeader,
						Value: fmt.Sprintf(".*%s.*%s.*", cookieKeyAndVal[0], cookieKeyAndVal[1]),
					},
				},
			}
			svcMatches, _ := gwr.mapRouteMatches(canary.Spec.Service.Match)
			mergedMatches := gwr.mergeMatchConditions([]v1.HTTPRouteMatch{cookieMatch}, svcMatches)
			stickyCanaryRouteRule.Matches = mergedMatches

			stickyCanaryRouteRule.Filters = append(stickyCanaryRouteRule.Filters, v1.HTTPRouteFilter{
				Type: v1.HTTPRouteFilterResponseHeaderModifier,
				ResponseHeaderModifier: &v1.HTTPHeaderFilter{
					Add: []v1.HTTPHeader{
						{
							Name:  setCookieHeader,
							Value: fmt.Sprintf("%s; %s=%d", previousCookie, maxAgeAttr, -1),
						},
					},
				},
			})
		}

		canary.Status.SessionAffinityCookie = ""
		return []v1.HTTPRouteRule{stickyCanaryRouteRule, *weightedRouteRule}, nil
	}
}

func (gwr *GatewayAPIRouter) mapRouteMatches(requestMatches []istiov1beta1.HTTPMatchRequest) ([]v1.HTTPRouteMatch, error) {
	matches := []v1.HTTPRouteMatch{}

	for _, requestMatch := range requestMatches {
		match := v1.HTTPRouteMatch{}
		if requestMatch.Uri != nil {
			if requestMatch.Uri.Regex != "" {
				match.Path = &v1.HTTPPathMatch{
					Type:  &pathMatchRegex,
					Value: &requestMatch.Uri.Regex,
				}
			} else if requestMatch.Uri.Exact != "" {
				match.Path = &v1.HTTPPathMatch{
					Type:  &pathMatchExact,
					Value: &requestMatch.Uri.Exact,
				}
			} else if requestMatch.Uri.Prefix != "" {
				match.Path = &v1.HTTPPathMatch{
					Type:  &pathMatchPrefix,
					Value: &requestMatch.Uri.Prefix,
				}
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified path matching selector: %+v\n", requestMatch.Uri)
			}
		}
		if requestMatch.Method != nil {
			if requestMatch.Method.Exact != "" {
				method := v1.HTTPMethod(requestMatch.Method.Exact)
				match.Method = &method
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified header matching selector: %+v\n", requestMatch.Headers)
			}
		}
		for key, val := range requestMatch.Headers {
			headerMatch := v1.HTTPHeaderMatch{}
			if val.Exact != "" {
				headerMatch.Name = v1.HTTPHeaderName(key)
				headerMatch.Type = &headerMatchExact
				headerMatch.Value = val.Exact
			} else if val.Regex != "" {
				headerMatch.Name = v1.HTTPHeaderName(key)
				headerMatch.Type = &headerMatchRegex
				headerMatch.Value = val.Regex
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified header matching selector: %+v\n", requestMatch.Headers)
			}
			if (v1.HTTPHeaderMatch{} != headerMatch) {
				match.Headers = append(match.Headers, headerMatch)
			}
		}

		for key, val := range requestMatch.QueryParams {
			queryMatch := v1.HTTPQueryParamMatch{}
			if val.Exact != "" {
				queryMatch.Name = v1.HTTPHeaderName(key)
				queryMatch.Type = &queryMatchExact
				queryMatch.Value = val.Exact
			} else if val.Regex != "" {
				queryMatch.Name = v1.HTTPHeaderName(key)
				queryMatch.Type = &queryMatchRegex
				queryMatch.Value = val.Regex
			} else {
				return nil, fmt.Errorf("Gateway API doesn't support the specified query matching selector: %+v\n", requestMatch.QueryParams)
			}

			if (v1.HTTPQueryParamMatch{} != queryMatch) {
				match.QueryParams = append(match.QueryParams, queryMatch)
			}
		}

		if !reflect.DeepEqual(match, v1.HTTPRouteMatch{}) {
			matches = append(matches, match)
		}
	}

	return matches, nil
}

func (gwr *GatewayAPIRouter) makeHTTPBackendRef(svcName string, weight, port int32, customBackend *flaggerv1.CustomBackend) v1.HTTPBackendRef {
	httpBackendRef := v1.HTTPBackendRef{
		BackendRef: v1.BackendRef{
			BackendObjectReference: v1.BackendObjectReference{
				Group: (*v1.Group)(&backendRefGroup),
				Kind:  (*v1.Kind)(&backendRefKind),
				Name:  v1.ObjectName(svcName),
				Port:  (*v1.PortNumber)(&port),
			},
			Weight: &weight,
		},
	}
	if customBackend != nil {
		if customBackend.BackendObjectReference != nil {
			httpBackendRef.BackendObjectReference = *customBackend.BackendObjectReference
		}
		if customBackend.Filters != nil {
			httpBackendRef.Filters = customBackend.Filters
		}
	}

	return httpBackendRef
}

func (gwr *GatewayAPIRouter) mergeMatchConditions(analysis, service []v1.HTTPRouteMatch) []v1.HTTPRouteMatch {
	if len(analysis) == 0 {
		return service
	}
	if len(service) == 0 {
		return analysis
	}

	merged := make([]v1.HTTPRouteMatch, len(service)*len(analysis))
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

func sortFiltersV1(headers []v1.HTTPHeader) {

	if headers != nil {
		slices.SortFunc(headers, func(a, b v1.HTTPHeader) int {
			if a.Name == b.Name {
				return strings.Compare(a.Value, b.Value)
			}
			return strings.Compare(string(a.Name), string(b.Name))
		})
	}
}

func (gwr *GatewayAPIRouter) makeFilters(canary *flaggerv1.Canary) []v1.HTTPRouteFilter {
	var filters []v1.HTTPRouteFilter

	if canary.Spec.Service.Headers != nil {

		if canary.Spec.Service.Headers.Request != nil {
			requestHeaderFilter := v1.HTTPRouteFilter{
				Type:                  v1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &v1.HTTPHeaderFilter{},
			}

			for name, val := range canary.Spec.Service.Headers.Request.Add {
				requestHeaderFilter.RequestHeaderModifier.Add = append(requestHeaderFilter.RequestHeaderModifier.Add, v1.HTTPHeader{
					Name:  v1.HTTPHeaderName(name),
					Value: val,
				})
			}
			sortFiltersV1(requestHeaderFilter.RequestHeaderModifier.Add)
			for name, val := range canary.Spec.Service.Headers.Request.Set {
				requestHeaderFilter.RequestHeaderModifier.Set = append(requestHeaderFilter.RequestHeaderModifier.Set, v1.HTTPHeader{
					Name:  v1.HTTPHeaderName(name),
					Value: val,
				})
			}

			sortFiltersV1(requestHeaderFilter.RequestHeaderModifier.Set)
			requestHeaderFilter.RequestHeaderModifier.Remove = append(requestHeaderFilter.RequestHeaderModifier.Remove, canary.Spec.Service.Headers.Request.Remove...)

			filters = append(filters, requestHeaderFilter)
		}
		if canary.Spec.Service.Headers.Response != nil {
			responseHeaderFilter := v1.HTTPRouteFilter{
				Type:                   v1.HTTPRouteFilterResponseHeaderModifier,
				ResponseHeaderModifier: &v1.HTTPHeaderFilter{},
			}

			for name, val := range canary.Spec.Service.Headers.Response.Add {
				responseHeaderFilter.ResponseHeaderModifier.Add = append(responseHeaderFilter.ResponseHeaderModifier.Add, v1.HTTPHeader{
					Name:  v1.HTTPHeaderName(name),
					Value: val,
				})
			}
			sortFiltersV1(responseHeaderFilter.ResponseHeaderModifier.Add)
			for name, val := range canary.Spec.Service.Headers.Response.Set {
				responseHeaderFilter.ResponseHeaderModifier.Set = append(responseHeaderFilter.ResponseHeaderModifier.Set, v1.HTTPHeader{
					Name:  v1.HTTPHeaderName(name),
					Value: val,
				})
			}
			sortFiltersV1(responseHeaderFilter.ResponseHeaderModifier.Set)

			responseHeaderFilter.ResponseHeaderModifier.Remove = append(responseHeaderFilter.ResponseHeaderModifier.Remove, canary.Spec.Service.Headers.Response.Remove...)

			filters = append(filters, responseHeaderFilter)
		}
	}

	if canary.Spec.Service.Rewrite != nil {
		rewriteFilter := v1.HTTPRouteFilter{
			Type:       v1.HTTPRouteFilterURLRewrite,
			URLRewrite: &v1.HTTPURLRewriteFilter{},
		}
		if canary.Spec.Service.Rewrite.Authority != "" {
			hostname := v1.PreciseHostname(canary.Spec.Service.Rewrite.Authority)
			rewriteFilter.URLRewrite.Hostname = &hostname
		}
		if canary.Spec.Service.Rewrite.Uri != "" {
			rewriteFilter.URLRewrite.Path = &v1.HTTPPathModifier{
				Type: v1.HTTPPathModifierType(canary.Spec.Service.Rewrite.GetType()),
			}
			if rewriteFilter.URLRewrite.Path.Type == v1.FullPathHTTPPathModifier {
				rewriteFilter.URLRewrite.Path.ReplaceFullPath = &canary.Spec.Service.Rewrite.Uri
			} else {
				rewriteFilter.URLRewrite.Path.ReplacePrefixMatch = &canary.Spec.Service.Rewrite.Uri
			}
		}

		filters = append(filters, rewriteFilter)
	}

	for _, mirror := range canary.Spec.Service.Mirror {
		mirror := mirror
		mirrorFilter := v1.HTTPRouteFilter{
			Type:          v1.HTTPRouteFilterRequestMirror,
			RequestMirror: toV1RequestMirrorFilter(mirror),
		}
		filters = append(filters, mirrorFilter)
	}

	if canary.Spec.Service.CorsPolicy != nil {
		corsFilter := v1.HTTPRouteFilter{
			Type: v1.HTTPRouteFilterCORS,
			CORS: gwr.toV1CORSFilter(canary.Spec.Service.CorsPolicy),
		}
		filters = append(filters, corsFilter)
	}

	return filters
}

func toV1RequestMirrorFilter(requestMirror v1beta1.HTTPRequestMirrorFilter) *v1.HTTPRequestMirrorFilter {
	return &v1.HTTPRequestMirrorFilter{
		BackendRef: v1.BackendObjectReference{
			Group:     (*v1.Group)(requestMirror.BackendRef.Group),
			Kind:      (*v1.Kind)(requestMirror.BackendRef.Kind),
			Namespace: (*v1.Namespace)(requestMirror.BackendRef.Namespace),
			Name:      (v1.ObjectName)(requestMirror.BackendRef.Name),
			Port:      (*v1.PortNumber)(requestMirror.BackendRef.Port),
		},
	}
}

func (gwr *GatewayAPIRouter) toV1CORSFilter(corsPolicy *istiov1beta1.CorsPolicy) *v1.HTTPCORSFilter {
	cors := &v1.HTTPCORSFilter{}

	// Note: CorsPolicy.AllowOrigins (StringMatch patterns) is not mapped because
	// Gateway API HTTPCORSFilter.AllowOrigins only supports simple origin strings,
	if len(corsPolicy.AllowOrigins) > 0 {
		gwr.logger.Errorf("'corsPolicy.allowOrigins' is not supported by Gateway API, use 'corsPolicy.allowOrigin' instead")
	}

	// Map AllowOrigin to AllowOrigins
	// not pattern matching like Istio's StringMatch type.
	if len(corsPolicy.AllowOrigin) > 0 {
		for _, origin := range corsPolicy.AllowOrigin {
			cors.AllowOrigins = append(cors.AllowOrigins, v1.CORSOrigin(origin))
		}
	}

	// Map AllowMethods
	if len(corsPolicy.AllowMethods) > 0 {
		for _, method := range corsPolicy.AllowMethods {
			cors.AllowMethods = append(cors.AllowMethods, v1.HTTPMethodWithWildcard(method))
		}
	}

	// Map AllowHeaders
	if len(corsPolicy.AllowHeaders) > 0 {
		for _, header := range corsPolicy.AllowHeaders {
			cors.AllowHeaders = append(cors.AllowHeaders, v1.HTTPHeaderName(header))
		}
	}

	// Map ExposeHeaders
	if len(corsPolicy.ExposeHeaders) > 0 {
		for _, header := range corsPolicy.ExposeHeaders {
			cors.ExposeHeaders = append(cors.ExposeHeaders, v1.HTTPHeaderName(header))
		}
	}

	// Map AllowCredentials
	if corsPolicy.AllowCredentials {
		allow := true
		cors.AllowCredentials = &allow
	}

	// Map MaxAge - convert duration string to seconds
	if corsPolicy.MaxAge != "" {
		// Parse duration string (e.g., "1d", "24h", "5s")
		duration, err := time.ParseDuration(corsPolicy.MaxAge)
		if err == nil {
			cors.MaxAge = int32(duration.Seconds())
		}
	}

	return cors
}

func toV1ParentRefs(gatewayRefs []v1beta1.ParentReference) []v1.ParentReference {
	parentRefs := make([]v1.ParentReference, 0)
	for i := range gatewayRefs {
		gatewayRef := gatewayRefs[i]
		parentRefs = append(parentRefs, v1.ParentReference{
			Group:       (*v1.Group)(gatewayRef.Group),
			Kind:        (*v1.Kind)(gatewayRef.Kind),
			Namespace:   (*v1.Namespace)(gatewayRef.Namespace),
			Name:        (v1.ObjectName)(gatewayRef.Name),
			SectionName: (*v1.SectionName)(gatewayRef.SectionName),
			Port:        (*v1.PortNumber)(gatewayRef.Port),
		})
	}
	return parentRefs
}

func checksum(data any) string {
	jsonBytes, _ := json.Marshal(data)
	hashBytes := sha256.Sum256(jsonBytes)

	return fmt.Sprintf("%x", hashBytes[:8])
}

// ensureReferenceGrants creates or updates ReferenceGrants required for cross-namespace backends.
func (gwr *GatewayAPIRouter) ensureReferenceGrants(canary *flaggerv1.Canary, httpRouteSpec v1.HTTPRouteSpec, hrNamespace string) error {
	referenceGrants := []*v1beta1.ReferenceGrant{}
	// Owner label value used across create/update/prune
	ownerLabelValue := fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)
	desired := map[string]struct{}{}
	for _, rule := range httpRouteSpec.Rules {
		for _, backendRef := range rule.BackendRefs {
			if backendRef.Namespace != nil {
				svcNamespace := string(*backendRef.Namespace)
				if svcNamespace != hrNamespace {
					group := v1beta1.Group("")
					kind := v1beta1.Kind("Service")
					if backendRef.Group != nil {
						group = v1beta1.Group(*backendRef.Group)
					}
					if backendRef.Kind != nil {
						kind = v1beta1.Kind(*backendRef.Kind)
					}
					name := (*v1beta1.ObjectName)(&backendRef.Name)

					// Build spec first so that checksum is based on rg.Spec (not canary.Spec).
					spec := v1beta1.ReferenceGrantSpec{
						From: []v1beta1.ReferenceGrantFrom{
							{
								Group:     "gateway.networking.k8s.io",
								Kind:      "HTTPRoute",
								Namespace: v1beta1.Namespace(hrNamespace),
							},
						},
						To: []v1beta1.ReferenceGrantTo{
							{
								Group: group,
								Kind:  kind,
								Name:  name,
							},
						},
					}

					rg := &v1beta1.ReferenceGrant{
						ObjectMeta: metav1.ObjectMeta{
							Name:      canary.Name,
							Namespace: svcNamespace,
							Annotations: map[string]string{
								hashAnnotation: checksum(spec),
							},
							Labels: map[string]string{
								ownerLabel: ownerLabelValue,
							},
						},
						Spec: spec,
					}

					referenceGrants = append(referenceGrants, rg)
					desired[fmt.Sprintf("%s/%s", rg.Namespace, rg.Name)] = struct{}{}
				}
			}
		}
	}

	for _, rg := range referenceGrants {
		realRg, err := gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(rg.Namespace).Get(context.TODO(), rg.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				_, err = gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(rg.Namespace).Create(context.TODO(), rg, metav1.CreateOptions{})
				if err == nil {
					gwr.logger.Infof("ReferenceGrant %s.%s has been created", rg.Name, rg.Namespace)
				} else if !errors.IsAlreadyExists(err) {
					return fmt.Errorf("ReferenceGrant %s.%s creation error: %w", rg.Name, rg.Namespace, err)
				}
			} else {
				return fmt.Errorf("ReferenceGrant %s.%s get error: %w", rg.Name, rg.Namespace, err)
			}
		} else {
			// Ensure annotations/labels are set and up to date.
			if realRg.Annotations == nil {
				realRg.Annotations = make(map[string]string)
			}
			if realRg.Labels == nil {
				realRg.Labels = make(map[string]string)
			}
			needsUpdate := false
			if realRg.Annotations[hashAnnotation] != checksum(rg.Spec) {
				realRg.Annotations[hashAnnotation] = checksum(rg.Spec)
				realRg.Spec = rg.Spec
				needsUpdate = true
			}
			if realRg.Labels[ownerLabel] != ownerLabelValue {
				realRg.Labels[ownerLabel] = ownerLabelValue
				needsUpdate = true
			}
			if needsUpdate {
				_, err = gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(rg.Namespace).Update(context.TODO(), realRg, metav1.UpdateOptions{})
				if err == nil {
					gwr.logger.Infof("ReferenceGrant %s.%s has been updated", rg.Name, rg.Namespace)
				} else if !errors.IsAlreadyExists(err) {
					return fmt.Errorf("ReferenceGrant %s.%s update error: %w", rg.Name, rg.Namespace, err)
				}
			}
		}
	}
	// Prune ReferenceGrants that belong to this canary but are no longer desired.
	rgs, err := gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(metav1.NamespaceAll).List(
		context.TODO(),
		metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", ownerLabel, ownerLabelValue)},
	)
	if err != nil {
		return fmt.Errorf("ReferenceGrants %s get error: %w", canary.Name, err)
	}
	for _, existing := range rgs.Items {
		key := fmt.Sprintf("%s/%s", existing.Namespace, existing.Name)
		if _, ok := desired[key]; !ok {
			// Delete stale grant; ignore NotFound to be idempotent.
			if err := gwr.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(existing.Namespace).Delete(
				context.TODO(), existing.Name, metav1.DeleteOptions{},
			); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("ReferenceGrant %s.%s delete error: %w", existing.Name, existing.Namespace, err)
			}
			gwr.logger.Infof("ReferenceGrant %s.%s has been deleted", existing.Name, existing.Namespace)
		}
	}
	return nil
}
