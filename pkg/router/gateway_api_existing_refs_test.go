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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
)

const (
	refsNamespace    = "default"
	ingressRouteName = "podinfo-ingress-route"
	svcRouteName     = "podinfo-svc-route"
)

func newTestGatewayAPICanaryWithRefs(routes ...string) *flaggerv1.Canary {
	c := newTestGatewayAPICanary()
	c.Spec.Service.GatewayRefs = nil
	c.Spec.Service.Timeout = ""
	c.Spec.Service.BackendTimeout = ""
	for _, r := range routes {
		c.Spec.Service.ExistingRefs = append(c.Spec.Service.ExistingRefs,
			flaggerv1.ExistingReference{Kind: flaggerv1.ExistingRefKindHTTPRoute, Name: r})
	}
	return c
}

func serviceBackend(name string, weight *int32, port int32) v1.HTTPBackendRef {
	return v1.HTTPBackendRef{
		BackendRef: v1.BackendRef{
			BackendObjectReference: v1.BackendObjectReference{
				Name: v1.ObjectName(name),
				Port: ptr.To(v1.PortNumber(port)),
			},
			Weight: weight,
		},
	}
}

func apexBackend() v1.HTTPBackendRef {
	return serviceBackend("podinfo", nil, 80)
}

func newIngressRouteFixture() *v1.HTTPRoute {
	return &v1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   refsNamespace,
			Name:        ingressRouteName,
			Labels:      map[string]string{"team": "ice"},
			Annotations: map[string]string{"channel": "experimental"},
		},
		Spec: v1.HTTPRouteSpec{
			CommonRouteSpec: v1.CommonRouteSpec{
				ParentRefs: []v1.ParentReference{
					{Name: "ingress-gw", Namespace: ptr.To(v1.Namespace("apps"))},
				},
			},
			Hostnames: []v1.Hostname{"podinfo.example.com"},
			Rules: []v1.HTTPRouteRule{
				{
					Name: ptr.To(v1.SectionName("podinfo")),
					Matches: []v1.HTTPRouteMatch{
						{Path: &v1.HTTPPathMatch{Type: ptr.To(v1.PathMatchPathPrefix), Value: ptr.To("/podinfo")}},
					},
					Filters: []v1.HTTPRouteFilter{
						{
							Type: v1.HTTPRouteFilterURLRewrite,
							URLRewrite: &v1.HTTPURLRewriteFilter{
								Path: &v1.HTTPPathModifier{Type: v1.PrefixMatchHTTPPathModifier, ReplacePrefixMatch: ptr.To("/")},
							},
						},
					},
					Timeouts:    &v1.HTTPRouteTimeouts{Request: ptr.To(v1.Duration("2500ms"))},
					Retry:       &v1.HTTPRouteRetry{Attempts: ptr.To(5)},
					BackendRefs: []v1.HTTPBackendRef{apexBackend()},
				},
			},
		},
	}
}

func newServiceRouteFixture() *v1.HTTPRoute {
	return &v1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: refsNamespace, Name: svcRouteName},
		Spec: v1.HTTPRouteSpec{
			CommonRouteSpec: v1.CommonRouteSpec{
				ParentRefs: []v1.ParentReference{
					{Group: ptr.To(v1.Group("")), Kind: ptr.To(v1.Kind("Service")), Name: "podinfo"},
				},
			},
			Rules: []v1.HTTPRouteRule{
				{
					Matches: []v1.HTTPRouteMatch{
						{Path: &v1.HTTPPathMatch{Type: ptr.To(v1.PathMatchPathPrefix), Value: ptr.To("/")}},
					},
					BackendRefs: []v1.HTTPBackendRef{apexBackend()},
				},
			},
		},
	}
}

func newRefsRouter(t *testing.T, canary *flaggerv1.Canary, routes ...*v1.HTTPRoute) *GatewayAPIRouter {
	t.Helper()
	mocks := newFixture(canary)
	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}
	for _, r := range routes {
		_, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes(refsNamespace).
			Create(context.TODO(), r.DeepCopy(), metav1.CreateOptions{})
		require.NoError(t, err)
	}
	return router
}

func getTestRoute(t *testing.T, router *GatewayAPIRouter, name string) *v1.HTTPRoute {
	t.Helper()
	r, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes(refsNamespace).
		Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)
	return r
}

func updateTestRoute(t *testing.T, router *GatewayAPIRouter, route *v1.HTTPRoute) {
	t.Helper()
	_, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes(refsNamespace).
		Update(context.TODO(), route, metav1.UpdateOptions{})
	require.NoError(t, err)
}

// backendWeights maps backend name to weight (-1 when the weight is unset).
func backendWeights(rule v1.HTTPRouteRule) map[string]int {
	out := map[string]int{}
	for _, b := range rule.BackendRefs {
		w := -1
		if b.Weight != nil {
			w = int(*b.Weight)
		}
		out[string(b.Name)] = w
	}
	return out
}

func requireWeights(t *testing.T, router *GatewayAPIRouter, routeName string, primary, canary int) {
	t.Helper()
	route := getTestRoute(t, router, routeName)
	require.NotEmpty(t, route.Spec.Rules)
	assert.Equal(t, map[string]int{"podinfo-primary": primary, "podinfo-canary": canary},
		backendWeights(route.Spec.Rules[0]), "route %s", routeName)
}

func TestGatewayAPIRouter_ExistingRefs_TakeOver(t *testing.T) {
	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName, svcRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture(), newServiceRouteFixture())

	require.NoError(t, router.Reconcile(canary))

	// Flagger must not create a route named after the apex Service.
	_, err := router.gatewayAPIClient.GatewayapiV1().HTTPRoutes(refsNamespace).
		Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.Error(t, err)

	for _, name := range []string{ingressRouteName, svcRouteName} {
		route := getTestRoute(t, router, name)
		refs := route.Spec.Rules[0].BackendRefs
		require.Len(t, refs, 2, name)
		assert.Equal(t, v1.ObjectName("podinfo-primary"), refs[0].Name)
		assert.Equal(t, v1.ObjectName("podinfo-canary"), refs[1].Name)
		assert.Equal(t, v1.PortNumber(80), *refs[0].Port)
		assert.Equal(t, v1.PortNumber(80), *refs[1].Port)
		requireWeights(t, router, name, 100, 0)
	}

	// the scheduler reconciles on every interval; weights set by SetRoutes must survive
	require.NoError(t, router.SetRoutes(canary, 70, 30, false))
	require.NoError(t, router.Reconcile(canary))
	requireWeights(t, router, ingressRouteName, 70, 30)
	requireWeights(t, router, svcRouteName, 70, 30)
}

func TestGatewayAPIRouter_ExistingRefs_Steps(t *testing.T) {
	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName, svcRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture(), newServiceRouteFixture())
	require.NoError(t, router.Reconcile(canary))

	for _, step := range [][2]int{{90, 10}, {50, 50}, {0, 100}, {100, 0}} {
		require.NoError(t, router.SetRoutes(canary, step[0], step[1], false))
		requireWeights(t, router, ingressRouteName, step[0], step[1])
		requireWeights(t, router, svcRouteName, step[0], step[1])

		p, c, mirrored, err := router.GetRoutes(canary)
		require.NoError(t, err)
		assert.Equal(t, step[0], p)
		assert.Equal(t, step[1], c)
		assert.False(t, mirrored)
	}
}

func TestGatewayAPIRouter_ExistingRefs_LeavesOtherFieldsAlone(t *testing.T) {
	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
	original := newIngressRouteFixture()
	router := newRefsRouter(t, canary, original)

	require.NoError(t, router.Reconcile(canary))
	require.NoError(t, router.SetRoutes(canary, 80, 20, false))

	got := getTestRoute(t, router, ingressRouteName)
	assert.Equal(t, original.Labels, got.Labels)
	assert.Equal(t, original.Annotations, got.Annotations)
	assert.Equal(t, original.Spec.ParentRefs, got.Spec.ParentRefs)
	assert.Equal(t, original.Spec.Hostnames, got.Spec.Hostnames)

	rule := got.Spec.Rules[0]
	assert.Equal(t, original.Spec.Rules[0].Name, rule.Name)
	assert.Equal(t, original.Spec.Rules[0].Matches, rule.Matches)
	assert.Equal(t, original.Spec.Rules[0].Filters, rule.Filters)
	assert.Equal(t, original.Spec.Rules[0].Timeouts, rule.Timeouts)
	assert.Equal(t, original.Spec.Rules[0].Retry, rule.Retry)
	assert.Equal(t, 5, *rule.Retry.Attempts)

	// putting the original backends back gives exactly the original spec
	restored := got.DeepCopy()
	restored.Spec.Rules[0].BackendRefs = original.Spec.Rules[0].BackendRefs
	assert.Equal(t, original.Spec, restored.Spec)
}

func TestGatewayAPIRouter_ExistingRefs_OtherBackendsAndRules(t *testing.T) {
	ingress := newIngressRouteFixture()
	ingress.Spec.Rules[0].BackendRefs = []v1.HTTPBackendRef{
		apexBackend(),
		serviceBackend("audit", ptr.To(int32(10)), 8080),
	}
	ingress.Spec.Rules = append(ingress.Spec.Rules, v1.HTTPRouteRule{
		Matches: []v1.HTTPRouteMatch{
			{Path: &v1.HTTPPathMatch{Type: ptr.To(v1.PathMatchPathPrefix), Value: ptr.To("/other")}},
		},
		BackendRefs: []v1.HTTPBackendRef{serviceBackend("ingress-gw-istio", nil, 80)},
	})

	t.Run("other backends and rules are kept", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		router := newRefsRouter(t, canary, ingress)
		require.NoError(t, router.Reconcile(canary))
		require.NoError(t, router.SetRoutes(canary, 60, 40, false))

		got := getTestRoute(t, router, ingressRouteName)
		require.Len(t, got.Spec.Rules, 2)
		assert.Equal(t, map[string]int{"podinfo-primary": 60, "podinfo-canary": 40, "audit": 10},
			backendWeights(got.Spec.Rules[0]))
		assert.Equal(t, ingress.Spec.Rules[1], got.Spec.Rules[1])
	})

	t.Run("route without a matching backend", func(t *testing.T) {
		other := newServiceRouteFixture()
		other.Name = "other-route"
		other.Spec.Rules[0].BackendRefs = []v1.HTTPBackendRef{serviceBackend("something-else", nil, 80)}

		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName, "other-route")
		router := newRefsRouter(t, canary, newIngressRouteFixture(), other)

		err := router.Reconcile(canary)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "other-route")
		assert.Contains(t, err.Error(), "no backendRef")

		// the valid route must not be changed when another one is misconfigured
		got := getTestRoute(t, router, ingressRouteName)
		assert.Equal(t, newIngressRouteFixture().Spec, got.Spec)
	})

	t.Run("missing route", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName, svcRouteName)
		router := newRefsRouter(t, canary, newIngressRouteFixture())

		err := router.Reconcile(canary)
		require.Error(t, err)
		assert.Contains(t, err.Error(), svcRouteName)

		got := getTestRoute(t, router, ingressRouteName)
		assert.Equal(t, newIngressRouteFixture().Spec, got.Spec)
	})
}

func TestGatewayAPIRouter_ExistingRefs_BackendFilters(t *testing.T) {
	route := newIngressRouteFixture()
	headers := &v1.HTTPHeaderFilter{Set: []v1.HTTPHeader{{Name: "x-mv", Value: "1"}}}
	route.Spec.Rules[0].BackendRefs[0].Filters = []v1.HTTPRouteFilter{
		{Type: v1.HTTPRouteFilterRequestHeaderModifier, RequestHeaderModifier: headers},
	}

	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
	router := newRefsRouter(t, canary, route)
	require.NoError(t, router.Reconcile(canary))

	refs := getTestRoute(t, router, ingressRouteName).Spec.Rules[0].BackendRefs
	require.Len(t, refs, 2)
	for _, ref := range refs {
		require.Len(t, ref.Filters, 1, string(ref.Name))
		assert.Equal(t, headers, ref.Filters[0].RequestHeaderModifier, string(ref.Name))
	}
}

func TestGatewayAPIRouter_ExistingRefs_Drift(t *testing.T) {
	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture())
	require.NoError(t, router.Reconcile(canary))
	require.NoError(t, router.SetRoutes(canary, 70, 30, false))

	t.Run("apex restored by terraform", func(t *testing.T) {
		route := getTestRoute(t, router, ingressRouteName)
		route.Spec.Rules[0].BackendRefs = []v1.HTTPBackendRef{apexBackend()}
		updateTestRoute(t, router, route)

		require.NoError(t, router.Reconcile(canary))
		requireWeights(t, router, ingressRouteName, 100, 0)
	})

	t.Run("weights changed by hand", func(t *testing.T) {
		route := getTestRoute(t, router, ingressRouteName)
		route.Spec.Rules[0].BackendRefs[0].Weight = ptr.To(int32(50))
		route.Spec.Rules[0].BackendRefs[1].Weight = ptr.To(int32(50))
		updateTestRoute(t, router, route)

		p, c, _, err := router.GetRoutes(canary)
		require.NoError(t, err)
		assert.Equal(t, 50, p)
		assert.Equal(t, 50, c)

		require.NoError(t, router.SetRoutes(canary, 70, 30, false))
		requireWeights(t, router, ingressRouteName, 70, 30)
	})
}

func TestGatewayAPIRouter_ExistingRefs_ParentNotAccepted(t *testing.T) {
	route := newIngressRouteFixture()
	route.Status.Parents = []v1.RouteParentStatus{
		{
			ParentRef:      route.Spec.ParentRefs[0],
			ControllerName: "istio.io/gateway-controller",
			Conditions: []metav1.Condition{
				{Type: string(v1.RouteConditionAccepted), Status: metav1.ConditionFalse, Reason: "NotAllowedByListeners"},
			},
		},
	}

	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
	router := newRefsRouter(t, canary, route)
	require.NoError(t, router.Reconcile(canary))

	_, _, _, err := router.GetRoutes(canary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ingressRouteName)
	assert.Contains(t, err.Error(), "not ready")
}

func TestGatewayAPIRouter_ExistingRefs_Finalize(t *testing.T) {
	route := newIngressRouteFixture()
	route.Spec.Rules[0].BackendRefs = []v1.HTTPBackendRef{
		apexBackend(),
		serviceBackend("audit", ptr.To(int32(10)), 8080),
	}

	t.Run("revertOnDeletion puts the apex back", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		canary.Spec.RevertOnDeletion = true
		router := newRefsRouter(t, canary, route)
		require.NoError(t, router.Reconcile(canary))
		require.NoError(t, router.SetRoutes(canary, 70, 30, false))

		require.NoError(t, router.Finalize(canary))
		got := getTestRoute(t, router, ingressRouteName)
		assert.Equal(t, map[string]int{"podinfo": -1, "audit": 10}, backendWeights(got.Spec.Rules[0]))
	})

	t.Run("without revertOnDeletion nothing changes", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		router := newRefsRouter(t, canary, route)
		require.NoError(t, router.Reconcile(canary))
		require.NoError(t, router.SetRoutes(canary, 70, 30, false))

		require.NoError(t, router.Finalize(canary))
		got := getTestRoute(t, router, ingressRouteName)
		assert.Equal(t, map[string]int{"podinfo-primary": 70, "podinfo-canary": 30, "audit": 10},
			backendWeights(got.Spec.Rules[0]))
	})

	t.Run("route already gone", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		canary.Spec.RevertOnDeletion = true
		router := newRefsRouter(t, canary)

		require.NoError(t, router.Finalize(canary))
	})
}

func TestGatewayAPIRouter_ExistingRefs_InvalidCombination(t *testing.T) {
	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
	canary.Spec.Service.Timeout = "10s"
	router := newRefsRouter(t, canary, newIngressRouteFixture())

	err := router.Reconcile(canary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	got := getTestRoute(t, router, ingressRouteName)
	assert.Equal(t, newIngressRouteFixture().Spec, got.Spec)
}

func TestGatewayAPIV1Beta1Router_ExistingRefs_NotSupported(t *testing.T) {
	canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
	mocks := newFixture(canary)
	router := &GatewayAPIV1Beta1Router{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "existingRefs")
	assert.Contains(t, err.Error(), "gatewayapi:v1")
}
