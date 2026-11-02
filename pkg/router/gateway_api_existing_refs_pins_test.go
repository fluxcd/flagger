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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
)

const pinHeader = "x-ice-podinfo-version"

func pinRequest(value string) []istiov1beta1.HTTPMatchRequest {
	return []istiov1beta1.HTTPMatchRequest{
		{Headers: map[string]istiov1alpha1.StringMatch{pinHeader: {Exact: value}}},
	}
}

func newTestCanaryWithPins(routes ...string) *flaggerv1.Canary {
	c := newTestGatewayAPICanaryWithRefs(routes...)
	c.Spec.Analysis.Match = pinRequest("canary")
	c.Spec.Analysis.PrimaryMatch = pinRequest("primary")
	return c
}

func headerExact(name, value string) v1.HTTPHeaderMatch {
	return v1.HTTPHeaderMatch{Name: v1.HTTPHeaderName(name), Type: ptr.To(v1.HeaderMatchExact), Value: value}
}

func ruleNames(route *v1.HTTPRoute) []string {
	names := make([]string, 0, len(route.Spec.Rules))
	for _, r := range route.Spec.Rules {
		if r.Name == nil {
			names = append(names, "")
			continue
		}
		names = append(names, string(*r.Name))
	}
	return names
}

func TestGatewayAPIRouter_ExistingRefs_PinRules(t *testing.T) {
	canary := newTestCanaryWithPins(ingressRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture())

	require.NoError(t, router.Reconcile(canary))

	route := getTestRoute(t, router, ingressRouteName)
	require.Len(t, route.Spec.Rules, 3)
	assert.Equal(t, []string{"podinfo-canary-pin", "podinfo-primary-pin", "podinfo"}, ruleNames(route))
	canaryRule, primaryRule, weighted := route.Spec.Rules[0], route.Spec.Rules[1], route.Spec.Rules[2]

	// the weighted rule keeps its match and has no header: untagged traffic follows the weights
	original := newIngressRouteFixture().Spec.Rules[0]
	assert.Equal(t, original.Matches, weighted.Matches)
	assert.Equal(t, map[string]int{"podinfo-primary": 100, "podinfo-canary": 0}, backendWeights(weighted))

	for _, tc := range []struct {
		name    string
		rule    v1.HTTPRouteRule
		header  string
		backend string
	}{
		{"canary pin", canaryRule, "canary", "podinfo-canary"},
		{"primary pin", primaryRule, "primary", "podinfo-primary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// the match of the route plus the header
			require.Len(t, tc.rule.Matches, 1)
			assert.Equal(t, original.Matches[0].Path, tc.rule.Matches[0].Path)
			assert.Equal(t, []v1.HTTPHeaderMatch{headerExact(pinHeader, tc.header)}, tc.rule.Matches[0].Headers)

			// one version only
			require.Len(t, tc.rule.BackendRefs, 1)
			assert.Equal(t, v1.ObjectName(tc.backend), tc.rule.BackendRefs[0].Name)
			assert.Equal(t, v1.PortNumber(80), *tc.rule.BackendRefs[0].Port)

			// the rest of the rule is copied: rewrite, timeouts, retry
			assert.Equal(t, original.Filters, tc.rule.Filters)
			assert.Equal(t, original.Timeouts, tc.rule.Timeouts)
			assert.Equal(t, original.Retry, tc.rule.Retry)
		})
	}

	// nothing else of the route changed
	assert.Equal(t, newIngressRouteFixture().Spec.Hostnames, route.Spec.Hostnames)
	assert.Equal(t, newIngressRouteFixture().Spec.ParentRefs, route.Spec.ParentRefs)
}

// At 100% the canary is not promoted yet and the primary has weight 0. The primary pin keeps it reachable.
func TestGatewayAPIRouter_ExistingRefs_PinRulesAtFullCanaryWeight(t *testing.T) {
	canary := newTestCanaryWithPins(ingressRouteName, svcRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture(), newServiceRouteFixture())
	require.NoError(t, router.Reconcile(canary))

	require.NoError(t, router.SetRoutes(canary, 0, 100, false))

	for _, name := range []string{ingressRouteName, svcRouteName} {
		route := getTestRoute(t, router, name)
		require.Len(t, route.Spec.Rules, 3, name)

		weighted := route.Spec.Rules[2]
		assert.Equal(t, map[string]int{"podinfo-primary": 0, "podinfo-canary": 100}, backendWeights(weighted), name)

		primaryRule := route.Spec.Rules[1]
		require.Len(t, primaryRule.BackendRefs, 1, name)
		assert.Equal(t, v1.ObjectName("podinfo-primary"), primaryRule.BackendRefs[0].Name, name)
		assert.Equal(t, []v1.HTTPHeaderMatch{headerExact(pinHeader, "primary")}, primaryRule.Matches[0].Headers, name)

		canaryRule := route.Spec.Rules[0]
		require.Len(t, canaryRule.BackendRefs, 1, name)
		assert.Equal(t, v1.ObjectName("podinfo-canary"), canaryRule.BackendRefs[0].Name, name)
	}
}

func TestGatewayAPIRouter_ExistingRefs_PinRulesAreStable(t *testing.T) {
	canary := newTestCanaryWithPins(ingressRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture())
	require.NoError(t, router.Reconcile(canary))
	require.NoError(t, router.SetRoutes(canary, 70, 30, false))
	before := getTestRoute(t, router, ingressRouteName).Spec.Rules

	// the scheduler reconciles on every interval: the pin rules must not be added again or reordered
	for i := 0; i < 3; i++ {
		require.NoError(t, router.Reconcile(canary))
	}
	require.NoError(t, router.SetRoutes(canary, 70, 30, false))
	after := getTestRoute(t, router, ingressRouteName).Spec.Rules
	assert.Equal(t, before, after)
	assert.Len(t, after, 3)

	// the weights are read from the weighted rule, not from a pin
	primary, canaryWeight, _, err := router.GetRoutes(canary)
	require.NoError(t, err)
	assert.Equal(t, 70, primary)
	assert.Equal(t, 30, canaryWeight)
}

func TestGatewayAPIRouter_ExistingRefs_PinRulesRemovedWithThePins(t *testing.T) {
	canary := newTestCanaryWithPins(ingressRouteName)
	router := newRefsRouter(t, canary, newIngressRouteFixture())
	require.NoError(t, router.Reconcile(canary))
	require.Len(t, getTestRoute(t, router, ingressRouteName).Spec.Rules, 3)

	canary.Spec.Analysis.Match = nil
	canary.Spec.Analysis.PrimaryMatch = nil
	require.NoError(t, router.SetRoutes(canary, 70, 30, false))

	route := getTestRoute(t, router, ingressRouteName)
	require.Len(t, route.Spec.Rules, 1)
	assert.Equal(t, []string{"podinfo"}, ruleNames(route))
	requireWeights(t, router, ingressRouteName, 70, 30)
}

func TestGatewayAPIRouter_ExistingRefs_PinRulesMatches(t *testing.T) {
	t.Run("a rule without matches gets the pins alone", func(t *testing.T) {
		route := newServiceRouteFixture()
		route.Spec.Rules[0].Matches = nil
		canary := newTestCanaryWithPins(svcRouteName)
		router := newRefsRouter(t, canary, route)

		require.NoError(t, router.Reconcile(canary))

		got := getTestRoute(t, router, svcRouteName)
		require.Len(t, got.Spec.Rules, 3)
		require.Len(t, got.Spec.Rules[0].Matches, 1)
		assert.Nil(t, got.Spec.Rules[0].Matches[0].Path)
		assert.Equal(t, []v1.HTTPHeaderMatch{headerExact(pinHeader, "canary")}, got.Spec.Rules[0].Matches[0].Headers)
		assert.Empty(t, got.Spec.Rules[2].Matches)
		// no rule name in the route: none is invented
		assert.Equal(t, []string{"", "", ""}, ruleNames(got))
	})

	t.Run("every match of the rule gets the pin, a header of the rule with the same name is replaced", func(t *testing.T) {
		route := newIngressRouteFixture()
		route.Spec.Rules[0].Matches = []v1.HTTPRouteMatch{
			{
				Path:    &v1.HTTPPathMatch{Type: ptr.To(v1.PathMatchPathPrefix), Value: ptr.To("/a")},
				Headers: []v1.HTTPHeaderMatch{headerExact("X-Team", "ice"), headerExact("X-Ice-Podinfo-Version", "other")},
			},
			{Path: &v1.HTTPPathMatch{Type: ptr.To(v1.PathMatchPathPrefix), Value: ptr.To("/b")}},
		}
		canary := newTestCanaryWithPins(ingressRouteName)
		router := newRefsRouter(t, canary, route)

		require.NoError(t, router.Reconcile(canary))

		got := getTestRoute(t, router, ingressRouteName)
		canaryMatches := got.Spec.Rules[0].Matches
		require.Len(t, canaryMatches, 2)
		assert.Equal(t, "/a", *canaryMatches[0].Path.Value)
		assert.Equal(t, []v1.HTTPHeaderMatch{headerExact(pinHeader, "canary"), headerExact("X-Team", "ice")}, canaryMatches[0].Headers)
		assert.Equal(t, "/b", *canaryMatches[1].Path.Value)
		assert.Equal(t, []v1.HTTPHeaderMatch{headerExact(pinHeader, "canary")}, canaryMatches[1].Headers)

		// the weighted rule is untouched
		assert.Equal(t, route.Spec.Rules[0].Matches, got.Spec.Rules[2].Matches)
	})
}

func TestGatewayAPIRouter_ExistingRefs_PinRulesOtherRulesAndBackends(t *testing.T) {
	route := newIngressRouteFixture()
	route.Spec.Rules[0].BackendRefs = []v1.HTTPBackendRef{
		apexBackend(),
		serviceBackend("audit", ptr.To(int32(10)), 8080),
	}
	other := v1.HTTPRouteRule{
		Name:        ptr.To(v1.SectionName("health")),
		Matches:     []v1.HTTPRouteMatch{{Path: &v1.HTTPPathMatch{Type: ptr.To(v1.PathMatchExact), Value: ptr.To("/healthz")}}},
		BackendRefs: []v1.HTTPBackendRef{serviceBackend("health", nil, 8080)},
	}
	route.Spec.Rules = append([]v1.HTTPRouteRule{other}, route.Spec.Rules...)
	canary := newTestCanaryWithPins(ingressRouteName)
	router := newRefsRouter(t, canary, route)

	require.NoError(t, router.Reconcile(canary))
	require.NoError(t, router.SetRoutes(canary, 70, 30, false))

	got := getTestRoute(t, router, ingressRouteName)
	// a rule without a canary backend gets no pins; the others come right before the weighted rule
	assert.Equal(t, []string{"health", "podinfo-canary-pin", "podinfo-primary-pin", "podinfo"}, ruleNames(got))
	assert.Equal(t, other, got.Spec.Rules[0])

	// a pin sends the request to its version only: the other backends of the weighted rule are not copied
	for _, i := range []int{1, 2} {
		require.Len(t, got.Spec.Rules[i].BackendRefs, 1)
	}
	assert.Equal(t, map[string]int{"podinfo-primary": 70, "podinfo-canary": 30, "audit": 10}, backendWeights(got.Spec.Rules[3]))
}

func TestGatewayAPIRouter_ExistingRefs_PinRulesFinalize(t *testing.T) {
	canary := newTestCanaryWithPins(ingressRouteName)
	canary.Spec.RevertOnDeletion = true
	router := newRefsRouter(t, canary, newIngressRouteFixture())
	require.NoError(t, router.Reconcile(canary))
	require.NoError(t, router.SetRoutes(canary, 0, 100, false))

	require.NoError(t, router.Finalize(canary))

	got := getTestRoute(t, router, ingressRouteName)
	require.Len(t, got.Spec.Rules, 1, "the pin rules go away with the canary Services")
	assert.Equal(t, map[string]int{"podinfo": -1}, backendWeights(got.Spec.Rules[0]))
	assert.Equal(t, newIngressRouteFixture().Spec.Rules[0].Matches, got.Spec.Rules[0].Matches)
}

func TestGatewayAPIRouter_ExistingRefs_PinRulesLongRuleName(t *testing.T) {
	long := string(make([]byte, 0))
	for len(long) < maxRuleNameLength {
		long += "a"
	}
	route := newIngressRouteFixture()
	route.Spec.Rules[0].Name = ptr.To(v1.SectionName(long))
	canary := newTestCanaryWithPins(ingressRouteName)
	router := newRefsRouter(t, canary, route)

	require.NoError(t, router.Reconcile(canary))

	got := getTestRoute(t, router, ingressRouteName)
	names := ruleNames(got)
	require.Len(t, names, 3)
	assert.LessOrEqual(t, len(names[0]), maxRuleNameLength)
	assert.LessOrEqual(t, len(names[1]), maxRuleNameLength)
	assert.NotEqual(t, names[0], names[1])
	assert.NotEqual(t, names[0], names[2])
}

func TestGatewayAPIRouter_ExistingRefs_PinsInvalid(t *testing.T) {
	canary := newTestCanaryWithPins(ingressRouteName)
	canary.Spec.Analysis.PrimaryMatch = nil
	router := newRefsRouter(t, canary, newIngressRouteFixture())

	err := router.Reconcile(canary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primaryMatch")

	got := getTestRoute(t, router, ingressRouteName)
	assert.Equal(t, newIngressRouteFixture().Spec, got.Spec)
}
