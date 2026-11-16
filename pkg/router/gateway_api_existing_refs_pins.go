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
	"fmt"
	"sort"
	"strings"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
)

// This file holds the pin rules of spec.service.existingRefs, see gateway_api_refs.go.

// isPinRule returns true for a rule that pins a request to one version: among the apex,
// primary and canary Services it has exactly one backendRef, and it is the primary or the
// canary. The weighted rule always has both of them and an untouched route has the apex, so
// neither is taken for a pin.
func (n canaryBackendNames) isPinRule(rule v1.HTTPRouteRule, routeNamespace string) bool {
	found, pinned := 0, false
	for _, ref := range rule.BackendRefs {
		if !n.isCanaryBackend(ref, routeNamespace) {
			continue
		}
		found++
		pinned = string(ref.Name) == n.primary || string(ref.Name) == n.canary
	}
	return found == 1 && pinned
}

// removePinRules drops every pin rule of the route.
func removePinRules(route *v1.HTTPRoute, names canaryBackendNames) {
	rules := make([]v1.HTTPRouteRule, 0, len(route.Spec.Rules))
	for _, rule := range route.Spec.Rules {
		if names.isPinRule(rule, route.Namespace) {
			continue
		}
		rules = append(rules, rule)
	}
	route.Spec.Rules = rules
}

// pinMatches maps spec.analysis.match (canary) and spec.analysis.primaryMatch (primary) to
// Gateway API matches. Both are nil when the Canary has no pins.
func (gwr *GatewayAPIRouter) pinMatches(canary *flaggerv1.Canary) (canaryPins, primaryPins []v1.HTTPRouteMatch, err error) {
	if !canary.HasExistingRefPins() {
		return nil, nil, nil
	}
	analysis := canary.GetAnalysis()
	if canaryPins, err = gwr.mapRouteMatches(analysis.Match); err != nil {
		return nil, nil, fmt.Errorf("spec.analysis.match: %w", err)
	}
	if primaryPins, err = gwr.mapRouteMatches(analysis.PrimaryMatch); err != nil {
		return nil, nil, fmt.Errorf("spec.analysis.primaryMatch: %w", err)
	}
	return canaryPins, primaryPins, nil
}

// insertPinRules puts, in front of every weighted rule of the route, one rule that pins a
// request to the canary and one that pins it to the primary. The pin rules are copies of the
// weighted rule (filters, timeouts, retry and the rest) with the pin merged into the matches
// and a single backendRef. Gateway API ranks a match with more header matches higher, so a
// request that carries a pin never reaches the weighted rule.
func (gwr *GatewayAPIRouter) insertPinRules(
	route *v1.HTTPRoute,
	names canaryBackendNames,
	port int32,
	canaryPins, primaryPins []v1.HTTPRouteMatch,
) {
	if len(canaryPins) == 0 || len(primaryPins) == 0 {
		return
	}

	rules := make([]v1.HTTPRouteRule, 0, len(route.Spec.Rules)*3)
	for _, rule := range route.Spec.Rules {
		hasPrimary, hasCanary := false, false
		var filters []v1.HTTPRouteFilter
		for _, ref := range rule.BackendRefs {
			if !names.isCanaryBackend(ref, route.Namespace) {
				continue
			}
			switch string(ref.Name) {
			case names.primary:
				hasPrimary = true
			case names.canary:
				hasCanary = true
				filters = ref.Filters
			}
		}
		if hasPrimary && hasCanary {
			rules = append(rules,
				gwr.pinRule(rule, canaryPins, names.canary, canaryPinRuleSuffix, filters, port),
				gwr.pinRule(rule, primaryPins, names.primary, primaryPinRuleSuffix, filters, port),
			)
		}
		rules = append(rules, rule)
	}
	route.Spec.Rules = rules
}

// pinRule copies the weighted rule into a rule that sends the requests of the pins to one Service.
func (gwr *GatewayAPIRouter) pinRule(
	weighted v1.HTTPRouteRule,
	pins []v1.HTTPRouteMatch,
	backend, suffix string,
	filters []v1.HTTPRouteFilter,
	port int32,
) v1.HTTPRouteRule {
	pinned := *weighted.DeepCopy()
	pinned.Matches = mergePinMatches(pins, weighted.Matches)
	pinned.BackendRefs = []v1.HTTPBackendRef{
		{BackendRef: gwr.makeBackendRef(backend, initialPrimaryWeight, port), Filters: copyFilters(filters)},
	}
	if weighted.Name != nil {
		name := pinRuleName(*weighted.Name, suffix)
		pinned.Name = &name
	}
	return pinned
}

// pinRuleName names a pin rule after its weighted rule: rule names are unique in a route.
func pinRuleName(name v1.SectionName, suffix string) v1.SectionName {
	base := string(name)
	if len(base)+len(suffix) > maxRuleNameLength {
		base = base[:maxRuleNameLength-len(suffix)]
	}
	return v1.SectionName(base + suffix)
}

// mergePinMatches adds every pin to every match of the weighted rule. A rule without matches
// matches everything, so the pins alone are its matches.
func mergePinMatches(pins, base []v1.HTTPRouteMatch) []v1.HTTPRouteMatch {
	if len(base) == 0 {
		base = []v1.HTTPRouteMatch{{}}
	}
	merged := make([]v1.HTTPRouteMatch, 0, len(pins)*len(base))
	for _, pin := range pins {
		for _, b := range base {
			m := *b.DeepCopy()
			m.Headers = mergeHeaderMatches(m.Headers, pin.Headers)
			m.QueryParams = mergeQueryParamMatches(m.QueryParams, pin.QueryParams)
			merged = append(merged, m)
		}
	}
	return merged
}

// mergeHeaderMatches returns the headers of the rule and of the pin, sorted by name, so the
// result does not change between passes. A pin wins over a header of the rule of the same name.
func mergeHeaderMatches(base, pin []v1.HTTPHeaderMatch) []v1.HTTPHeaderMatch {
	if len(pin) == 0 {
		return base
	}
	out := make([]v1.HTTPHeaderMatch, 0, len(base)+len(pin))
	for _, b := range base {
		overridden := false
		for _, p := range pin {
			if strings.EqualFold(string(b.Name), string(p.Name)) {
				overridden = true
				break
			}
		}
		if !overridden {
			out = append(out, b)
		}
	}
	out = append(out, pin...)
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(string(out[i].Name)) < strings.ToLower(string(out[j].Name))
	})
	return out
}

// mergeQueryParamMatches is mergeHeaderMatches for query parameters, whose names are case sensitive.
func mergeQueryParamMatches(base, pin []v1.HTTPQueryParamMatch) []v1.HTTPQueryParamMatch {
	if len(pin) == 0 {
		return base
	}
	out := make([]v1.HTTPQueryParamMatch, 0, len(base)+len(pin))
	for _, b := range base {
		overridden := false
		for _, p := range pin {
			if b.Name == p.Name {
				overridden = true
				break
			}
		}
		if !overridden {
			out = append(out, b)
		}
	}
	out = append(out, pin...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
