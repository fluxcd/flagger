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
	"fmt"
	"strings"
	"testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGatewayAPIRouter_Reconcile(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	httpRoute, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	routeRules := httpRoute.Spec.Rules
	require.Equal(t, len(routeRules), 1)

	backendRefs := routeRules[0].BackendRefs
	require.Equal(t, len(backendRefs), 2)
	assert.Equal(t, int32(100), *backendRefs[0].Weight)
	assert.Equal(t, int32(0), *backendRefs[1].Weight)

	timeout := routeRules[0].Timeouts
	assert.Equal(t, string(*timeout.Request), canary.Spec.Service.Timeout)

	// assert that http route annotations injected by the networking controller is preserved.
	httpRoute.Annotations["foo"] = "bar"
	_, err = router.gatewayAPIClient.GatewayapiV1().HTTPRoutes("default").Update(context.TODO(), httpRoute, metav1.UpdateOptions{})
	require.NoError(t, err)
	err = router.Reconcile(canary)
	require.NoError(t, err)

	httpRoute, err = router.gatewayAPIClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, httpRoute.Annotations["foo"], "bar")
}

func TestGatewayAPIRouter_Routes(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	t.Run("normal", func(t *testing.T) {
		err = router.SetRoutes(canary, 50, 50, false)
		require.NoError(t, err)

		httpRoute, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		primary := httpRoute.Spec.Rules[0].BackendRefs[0]
		assert.Equal(t, int32(50), *primary.Weight)
	})

	t.Run("session affinity", func(t *testing.T) {
		canary := mocks.canary.DeepCopy()
		cookieKey := "flagger-cookie"
		// enable session affinity and start canary run
		canary.Spec.Analysis.SessionAffinity = &flaggerv1.SessionAffinity{
			CookieName: cookieKey,
			MaxAge:     300,
		}
		_, pSvcName, cSvcName := canary.GetServiceNames()

		err := router.SetRoutes(canary, 90, 10, false)

		hr, err := mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, hr.Spec.Rules, 2)

		stickyRule := hr.Spec.Rules[0]
		weightedRule := hr.Spec.Rules[1]

		// stickyRoute should match against a cookie and direct all traffic to the canary when a canary run is active.
		cookieMatch := stickyRule.Matches[0].Headers[0]
		assert.Equal(t, *cookieMatch.Type, v1.HeaderMatchRegularExpression)
		assert.Equal(t, string(cookieMatch.Name), cookieHeader)
		assert.Contains(t, cookieMatch.Value, cookieKey)

		assert.Equal(t, len(stickyRule.BackendRefs), 2)
		for _, backendRef := range stickyRule.BackendRefs {
			if string(backendRef.BackendRef.Name) == pSvcName {
				assert.Equal(t, *backendRef.BackendRef.Weight, int32(0))
			}
			if string(backendRef.BackendRef.Name) == cSvcName {
				assert.Equal(t, *backendRef.BackendRef.Weight, int32(100))
			}
		}

		// weightedRoute should do regular weight based routing and inject the Set-Cookie header
		// for all responses returned from the canary deployment.
		var found bool
		for _, backendRef := range weightedRule.BackendRefs {
			if string(backendRef.Name) == cSvcName {
				found = true
				filter := backendRef.Filters[0]
				assert.Equal(t, filter.Type, v1.HTTPRouteFilterResponseHeaderModifier)
				assert.NotNil(t, filter.ResponseHeaderModifier)
				assert.Equal(t, string(filter.ResponseHeaderModifier.Add[0].Name), setCookieHeader)
				assert.Equal(t, filter.ResponseHeaderModifier.Add[0].Value, fmt.Sprintf("%s; %s=%d", canary.Status.SessionAffinityCookie, maxAgeAttr, 300))
				assert.Equal(t, *backendRef.Weight, int32(10))
			}
			if string(backendRef.Name) == pSvcName {
				assert.Equal(t, *backendRef.Weight, int32(90))
			}
		}
		assert.True(t, found)
		assert.True(t, strings.HasPrefix(canary.Status.SessionAffinityCookie, cookieKey))

		// reconcile Canary and HTTPRoute
		err = router.Reconcile(canary)
		require.NoError(t, err)

		// HTTPRoute should be unchanged
		hr, err = mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, hr.Spec.Rules, 2)
		assert.Empty(t, cmp.Diff(hr.Spec.Rules[0], stickyRule))
		assert.Empty(t, cmp.Diff(hr.Spec.Rules[1], weightedRule))

		// further continue the canary run
		err = router.SetRoutes(canary, 50, 50, false)
		require.NoError(t, err)
		hr, err = mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		stickyRule = hr.Spec.Rules[0]
		weightedRule = hr.Spec.Rules[1]

		// stickyRoute should match against a cookie and direct all traffic to the canary when a canary run is active.
		cookieMatch = stickyRule.Matches[0].Headers[0]
		assert.Equal(t, *cookieMatch.Type, v1.HeaderMatchRegularExpression)
		assert.Equal(t, string(cookieMatch.Name), cookieHeader)
		assert.Contains(t, cookieMatch.Value, cookieKey)

		assert.Equal(t, len(stickyRule.BackendRefs), 2)
		for _, backendRef := range stickyRule.BackendRefs {
			if string(backendRef.BackendRef.Name) == pSvcName {
				assert.Equal(t, *backendRef.BackendRef.Weight, int32(0))
			}
			if string(backendRef.BackendRef.Name) == cSvcName {
				assert.Equal(t, *backendRef.BackendRef.Weight, int32(100))
			}
		}

		// weightedRoute should do regular weight based routing and inject the Set-Cookie header
		// for all responses returned from the canary deployment.
		found = false
		for _, backendRef := range weightedRule.BackendRefs {
			if string(backendRef.Name) == cSvcName {
				found = true
				filter := backendRef.Filters[0]
				assert.Equal(t, filter.Type, v1.HTTPRouteFilterResponseHeaderModifier)
				assert.NotNil(t, filter.ResponseHeaderModifier)
				assert.Equal(t, string(filter.ResponseHeaderModifier.Add[0].Name), setCookieHeader)
				assert.Equal(t, filter.ResponseHeaderModifier.Add[0].Value, fmt.Sprintf("%s; %s=%d", canary.Status.SessionAffinityCookie, maxAgeAttr, 300))

				assert.Equal(t, *backendRef.Weight, int32(50))
			}
			if string(backendRef.Name) == pSvcName {
				assert.Equal(t, *backendRef.Weight, int32(50))
			}
		}
		assert.True(t, found)

		// promotion
		err = router.SetRoutes(canary, 100, 0, false)
		require.NoError(t, err)
		hr, err = mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Empty(t, canary.Status.SessionAffinityCookie)
		assert.Contains(t, canary.Status.PreviousSessionAffinityCookie, cookieKey)

		stickyRule = hr.Spec.Rules[0]
		weightedRule = hr.Spec.Rules[1]

		// Assert that the stucky rule matches against the previous cookie and tells clients to delete it.
		cookieMatch = stickyRule.Matches[0].Headers[0]
		assert.Equal(t, *cookieMatch.Type, v1.HeaderMatchRegularExpression)
		assert.Equal(t, string(cookieMatch.Name), cookieHeader)
		assert.Contains(t, cookieMatch.Value, cookieKey)

		assert.Equal(t, stickyRule.Filters[0].Type, v1.HTTPRouteFilterResponseHeaderModifier)
		headerModifier := stickyRule.Filters[0].ResponseHeaderModifier
		assert.NotNil(t, headerModifier)
		assert.Equal(t, string(headerModifier.Add[0].Name), setCookieHeader)
		assert.Equal(t, headerModifier.Add[0].Value, fmt.Sprintf("%s; %s=%d", canary.Status.PreviousSessionAffinityCookie, maxAgeAttr, -1))

		for _, backendRef := range stickyRule.BackendRefs {
			if string(backendRef.BackendRef.Name) == pSvcName {
				assert.Equal(t, *backendRef.BackendRef.Weight, int32(100))
			}
			if string(backendRef.BackendRef.Name) == cSvcName {
				assert.Equal(t, *backendRef.BackendRef.Weight, int32(0))
			}
		}

		for _, backendRef := range weightedRule.BackendRefs {
			if string(backendRef.Name) == cSvcName {
				// Assert the weighted rule does not send Set-Cookie headers anymore
				assert.Len(t, backendRef.Filters, 0)
				assert.Equal(t, *backendRef.Weight, int32(0))
			}
			if string(backendRef.Name) == pSvcName {
				assert.Equal(t, *backendRef.Weight, int32(100))
			}
		}
		assert.True(t, found)
	})

	t.Run("b/g mirror", func(t *testing.T) {
		canary := mocks.canary.DeepCopy()
		canary.Spec.Analysis.Mirror = true
		canary.Spec.Analysis.Iterations = 5
		_, _, cSvcName := canary.GetServiceNames()

		err = router.SetRoutes(canary, 100, 0, true)
		hr, err := mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, hr.Spec.Rules, 1)

		rule := hr.Spec.Rules[0]
		var found bool
		for _, filter := range rule.Filters {
			if filter.Type == v1.HTTPRouteFilterRequestMirror && filter.RequestMirror != nil &&
				string(filter.RequestMirror.BackendRef.Name) == cSvcName {
				found = true
			}
		}
		assert.True(t, found, "could not find request mirror filter in HTTPRoute")

		// Mark the status as progressing to assert that request mirror filter is ignored.
		canary.Status.Phase = flaggerv1.CanaryPhaseProgressing
		err = router.Reconcile(canary)
		require.NoError(t, err)

		hr, err = mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, hr.Spec.Rules, 1)
		assert.Empty(t, cmp.Diff(hr.Spec.Rules[0], rule))

		err = router.SetRoutes(canary, 100, 0, false)
		hr, err = mocks.meshClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, hr.Spec.Rules, 1)
		assert.Len(t, hr.Spec.Rules[0].Filters, 0)
	})

	t.Run("custom backend filters", func(t *testing.T) {
		canary := mocks.canary.DeepCopy()
		primaryHostName := v1.PreciseHostname("primary.example.com")
		canary.Spec.Service.Primary = &flaggerv1.CustomBackend{
			Filters: []v1.HTTPRouteFilter{
				{
					Type: v1.HTTPRouteFilterURLRewrite,
					URLRewrite: &v1.HTTPURLRewriteFilter{
						Hostname: &primaryHostName,
					},
				},
			},
		}

		name := v1.ObjectName("canary")
		unmanagedSvcNamespace := "kube-system"
		namespace := v1.Namespace(unmanagedSvcNamespace)
		port := v1.PortNumber(30080)
		objRef := v1.BackendObjectReference{
			Name:      name,
			Namespace: &namespace,
			Port:      &port,
		}

		canary.Spec.Service.Canary = &flaggerv1.CustomBackend{
			BackendObjectReference: &objRef,
		}
		err = router.SetRoutes(canary, 50, 50, false)
		require.NoError(t, err)

		httpRoute, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		primary := httpRoute.Spec.Rules[0].BackendRefs[0]
		assert.Equal(t, int32(50), *primary.Weight)

		canaryBackend := httpRoute.Spec.Rules[0].BackendRefs[1]
		assert.Equal(t, canaryBackend.Name, name)
		assert.Equal(t, canaryBackend.Namespace, &namespace)
		assert.Equal(t, canaryBackend.Port, &port)

		primaryBackend := httpRoute.Spec.Rules[0].BackendRefs[0].Filters[0].URLRewrite
		assert.Equal(t, primaryBackend.Hostname, &primaryHostName)

		err = router.Reconcile(canary)
		require.NoError(t, err)

		referenceGrant, err := router.gatewayAPIClient.GatewayapiV1beta1().ReferenceGrants(unmanagedSvcNamespace).Get(context.TODO(), canary.Name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, unmanagedSvcNamespace, string(referenceGrant.Namespace))
		assert.Equal(t, "HTTPRoute", string(referenceGrant.Spec.From[0].Kind))
		assert.Equal(t, canary.Namespace, string(referenceGrant.Spec.From[0].Namespace))
		assert.Equal(t, "Service", string(referenceGrant.Spec.To[0].Kind))
		assert.Equal(t, "", string(referenceGrant.Spec.To[0].Group))
		assert.Equal(t, string(name), string(*referenceGrant.Spec.To[0].Name))
	})
}

func TestGatewayAPIRouter_getSessionAffinityRouteRules(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	cookieKey := "flagger-cookie"
	canary.Spec.Analysis.SessionAffinity = &flaggerv1.SessionAffinity{
		CookieName: cookieKey,
		MaxAge:     300,
	}

	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}
	_, pSvcName, cSvcName := canary.GetServiceNames()
	weightedRouteRule := &v1.HTTPRouteRule{
		BackendRefs: []v1.HTTPBackendRef{
			router.makeHTTPBackendRef(pSvcName, initialPrimaryWeight, canary.Spec.Service.Port, canary.Spec.Service.Primary),
			router.makeHTTPBackendRef(cSvcName, initialCanaryWeight, canary.Spec.Service.Port, canary.Spec.Service.Canary),
		},
	}
	rules, err := router.getSessionAffinityRouteRules(canary, 10, weightedRouteRule)
	require.NoError(t, err)
	assert.Equal(t, len(rules), 2)
	assert.True(t, strings.HasPrefix(canary.Status.SessionAffinityCookie, cookieKey))

	stickyRule := rules[0]
	cookieMatch := stickyRule.Matches[0].Headers[0]
	assert.Equal(t, *cookieMatch.Type, v1.HeaderMatchRegularExpression)
	assert.Equal(t, string(cookieMatch.Name), cookieHeader)
	assert.Contains(t, cookieMatch.Value, cookieKey)

	assert.Equal(t, len(stickyRule.BackendRefs), 2)
	for _, backendRef := range stickyRule.BackendRefs {
		if string(backendRef.BackendRef.Name) == pSvcName {
			assert.Equal(t, *backendRef.BackendRef.Weight, int32(0))
		}
		if string(backendRef.BackendRef.Name) == cSvcName {
			assert.Equal(t, *backendRef.BackendRef.Weight, int32(100))
		}
	}

	weightedRule := rules[1]
	var found bool
	for _, backendRef := range weightedRule.BackendRefs {
		if string(backendRef.Name) == cSvcName {
			found = true
			filter := backendRef.Filters[0]
			assert.Equal(t, filter.Type, v1.HTTPRouteFilterResponseHeaderModifier)
			assert.NotNil(t, filter.ResponseHeaderModifier)
			assert.Equal(t, string(filter.ResponseHeaderModifier.Add[0].Name), setCookieHeader)
			assert.Equal(t, filter.ResponseHeaderModifier.Add[0].Value, fmt.Sprintf("%s; %s=%d", canary.Status.SessionAffinityCookie, maxAgeAttr, 300))
		}
	}
	assert.True(t, found)

	rules, err = router.getSessionAffinityRouteRules(canary, 0, weightedRouteRule)
	assert.Empty(t, canary.Status.SessionAffinityCookie)
	assert.Contains(t, canary.Status.PreviousSessionAffinityCookie, cookieKey)

	stickyRule = rules[0]
	cookieMatch = stickyRule.Matches[0].Headers[0]
	assert.Equal(t, *cookieMatch.Type, v1.HeaderMatchRegularExpression)
	assert.Equal(t, string(cookieMatch.Name), cookieHeader)
	assert.Contains(t, cookieMatch.Value, cookieKey)

	assert.Equal(t, stickyRule.Filters[0].Type, v1.HTTPRouteFilterResponseHeaderModifier)
	headerModifier := stickyRule.Filters[0].ResponseHeaderModifier
	assert.NotNil(t, headerModifier)
	assert.Equal(t, string(headerModifier.Add[0].Name), setCookieHeader)
	assert.Equal(t, headerModifier.Add[0].Value, fmt.Sprintf("%s; %s=%d", canary.Status.PreviousSessionAffinityCookie, maxAgeAttr, -1))
}

func TestGatewayAPIRouter_makeFilters(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	canary.Spec.Service.Headers = &istiov1beta1.Headers{
		Response: &istiov1beta1.HeaderOperations{
			Set: map[string]string{"h1": "v1", "h2": "v2", "h3": "v3"},
			Add: map[string]string{"h1": "v1", "h2": "v2", "h3": "v3"},
		},
		Request: &istiov1beta1.HeaderOperations{
			Set: map[string]string{"h1": "v1", "h2": "v2", "h3": "v3"},
			Add: map[string]string{"h1": "v1", "h2": "v2", "h3": "v3"},
		},
	}

	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	ignoreCmpOptions := []cmp.Option{
		cmpopts.IgnoreFields(v1.BackendRef{}, "Weight"),
		cmpopts.EquateEmpty(),
	}

	filters := router.makeFilters(canary)

	for i := 0; i < 10; i++ {
		newFilters := router.makeFilters(canary)
		filtersDiff := cmp.Diff(
			filters, newFilters,
			ignoreCmpOptions...,
		)
		assert.Equal(t, "", filtersDiff)
	}
}
