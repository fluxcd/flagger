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
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
)

// This file implements spec.service.existingRefs for the gatewayapi:v1 provider.
//
// Instead of generating an HTTPRoute named after the apex Service, Flagger manages the
// weights of HTTPRoutes that already exist. In every rule of a referenced route that has a
// backendRef to the apex, primary or canary Service, the backendRefs are replaced by exactly
// two (primary and canary) with the computed weights. Everything else in the route is left
// alone: parentRefs, hostnames, matches, filters, timeouts, retry, other rules and backendRefs
// to other Services.
//
// With spec.analysis.match (canary) and spec.analysis.primaryMatch (primary) every such weighted
// rule is also copied twice, with the pin merged into the copy's matches:
//
//	rule 1  match + canary pin   -> canary only      (filters, timeouts, retry copied)
//	rule 2  match + primary pin  -> primary only
//	rule 3  match                -> primary + canary (the weighted rule, unchanged)
//
// A request that carries a pin goes to that version whatever the weights are, e.g. to the
// primary while the canary is at 100% and not promoted yet. A request without a pin follows
// the weights. The pin rules are rebuilt from the weighted rule on every pass.

const (
	canaryPinRuleSuffix  = "-canary-pin"
	primaryPinRuleSuffix = "-primary-pin"
	// maxRuleNameLength is the maximum length of a rule name (SectionName).
	maxRuleNameLength = 253
)

// canaryBackendNames are the Services that Flagger moves traffic between.
type canaryBackendNames struct {
	apex    string
	primary string
	canary  string
}

// isCanaryBackend returns true when the backendRef is a core Service named apex, primary or
// canary, in the namespace of the route.
func (n canaryBackendNames) isCanaryBackend(ref v1.HTTPBackendRef, routeNamespace string) bool {
	if ref.Group != nil && *ref.Group != "" {
		return false
	}
	if ref.Kind != nil && *ref.Kind != "Service" {
		return false
	}
	if ref.Namespace != nil && string(*ref.Namespace) != routeNamespace {
		return false
	}
	name := string(ref.Name)
	return name == n.apex || name == n.primary || name == n.canary
}

func copyFilters(in []v1.HTTPRouteFilter) []v1.HTTPRouteFilter {
	if in == nil {
		return nil
	}
	out := make([]v1.HTTPRouteFilter, len(in))
	for i := range in {
		in[i].DeepCopyInto(&out[i])
	}
	return out
}

// applyCanaryBackends rewrites the canary backends of every rule of the route.
// When primaryWeight and canaryWeight are nil the weights already in the route are kept
// (primary 100 / canary 0 when the rule still points at the apex Service). It returns
// whether the route changed and whether at least one rule had a canary backend.
func (gwr *GatewayAPIRouter) applyCanaryBackends(
	route *v1.HTTPRoute,
	names canaryBackendNames,
	port int32,
	primaryWeight, canaryWeight *int32,
) (changed bool, matched bool) {
	for i := range route.Spec.Rules {
		rule := &route.Spec.Rules[i]

		first := -1
		var filters []v1.HTTPRouteFilter
		var currentPrimary, currentCanary *int32
		for j, ref := range rule.BackendRefs {
			if !names.isCanaryBackend(ref, route.Namespace) {
				continue
			}
			if first == -1 {
				first = j
				filters = ref.Filters
			}
			switch string(ref.Name) {
			case names.primary:
				currentPrimary = ref.Weight
			case names.canary:
				currentCanary = ref.Weight
			}
		}
		if first == -1 {
			continue
		}
		matched = true

		pw, cw := initialPrimaryWeight, initialCanaryWeight
		switch {
		case primaryWeight != nil && canaryWeight != nil:
			pw, cw = *primaryWeight, *canaryWeight
		case currentPrimary != nil && currentCanary != nil:
			pw, cw = *currentPrimary, *currentCanary
		}

		backendRefs := make([]v1.HTTPBackendRef, 0, len(rule.BackendRefs))
		for j, ref := range rule.BackendRefs {
			switch {
			case j == first:
				backendRefs = append(backendRefs,
					v1.HTTPBackendRef{BackendRef: gwr.makeBackendRef(names.primary, pw, port), Filters: copyFilters(filters)},
					v1.HTTPBackendRef{BackendRef: gwr.makeBackendRef(names.canary, cw, port), Filters: copyFilters(filters)},
				)
			case names.isCanaryBackend(ref, route.Namespace):
				// folded into the two entries above
			default:
				backendRefs = append(backendRefs, ref)
			}
		}

		if !equality.Semantic.DeepEqual(rule.BackendRefs, backendRefs) {
			rule.BackendRefs = backendRefs
			changed = true
		}
	}
	return changed, matched
}

// revertCanaryBackends puts the apex Service back as the only canary backend of every rule.
func (gwr *GatewayAPIRouter) revertCanaryBackends(
	route *v1.HTTPRoute,
	names canaryBackendNames,
	port int32,
) (changed bool) {
	for i := range route.Spec.Rules {
		rule := &route.Spec.Rules[i]

		first := -1
		var filters []v1.HTTPRouteFilter
		for j, ref := range rule.BackendRefs {
			if names.isCanaryBackend(ref, route.Namespace) {
				first = j
				filters = ref.Filters
				break
			}
		}
		if first == -1 {
			continue
		}

		apex := gwr.makeBackendRef(names.apex, 0, port)
		apex.Weight = nil
		backendRefs := make([]v1.HTTPBackendRef, 0, len(rule.BackendRefs))
		for j, ref := range rule.BackendRefs {
			switch {
			case j == first:
				backendRefs = append(backendRefs, v1.HTTPBackendRef{BackendRef: apex, Filters: copyFilters(filters)})
			case names.isCanaryBackend(ref, route.Namespace):
			default:
				backendRefs = append(backendRefs, ref)
			}
		}

		if !equality.Semantic.DeepEqual(rule.BackendRefs, backendRefs) {
			rule.BackendRefs = backendRefs
			changed = true
		}
	}
	return changed
}

// httpRouteParentsReady returns an error if a parent of the route is not accepted or has not
// observed the current generation (the same rule the generated route uses).
func httpRouteParentsReady(route *v1.HTTPRoute) error {
	generation := route.GetGeneration()
	for _, parentRef := range route.Spec.CommonRouteSpec.ParentRefs {
		for _, parentStatus := range route.Status.Parents {
			if !reflect.DeepEqual(parentStatus.ParentRef, parentRef) {
				continue
			}
			for _, condition := range parentStatus.Conditions {
				if condition.Type == string(v1.RouteConditionAccepted) &&
					(condition.Status != metav1.ConditionTrue || condition.ObservedGeneration < generation) {
					return fmt.Errorf(
						"HTTPRoute %s.%s parent %s is not ready (status: %s, observed generation: %d, current generation: %d)",
						route.Name, route.Namespace, parentRef.Name, string(condition.Status), condition.ObservedGeneration, generation)
				}
			}
		}
	}
	return nil
}

func canaryBackendNamesOf(canary *flaggerv1.Canary) canaryBackendNames {
	apex, primary, canarySvc := canary.GetServiceNames()
	return canaryBackendNames{apex: apex, primary: primary, canary: canarySvc}
}

func (gwr *GatewayAPIRouter) getRefRoute(canary *flaggerv1.Canary, ref flaggerv1.ExistingReference) (*v1.HTTPRoute, error) {
	if ref.Kind != flaggerv1.ExistingRefKindHTTPRoute {
		return nil, fmt.Errorf("spec.service.existingRefs: kind %q is not supported, supported: %s",
			ref.Kind, flaggerv1.ExistingRefKindHTTPRoute)
	}
	route, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(canary.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("HTTPRoute %s.%s get error: %w", ref.Name, canary.Namespace, err)
	}
	return route, nil
}

// reconcileExistingRefs takes over the referenced routes: the apex backendRef becomes the
// primary/canary pair. Weights already in a route are kept, because the scheduler calls
// Reconcile on every interval.
func (gwr *GatewayAPIRouter) reconcileExistingRefs(canary *flaggerv1.Canary) error {
	if err := canary.ValidateExistingRefs(); err != nil {
		return err
	}
	return gwr.updateExistingRefs(canary, nil, nil)
}

// setRoutesOnExistingRefs sets the same weights on every referenced route.
func (gwr *GatewayAPIRouter) setRoutesOnExistingRefs(canary *flaggerv1.Canary, primaryWeight, canaryWeight int) error {
	pw, cw := int32(primaryWeight), int32(canaryWeight)
	return gwr.updateExistingRefs(canary, &pw, &cw)
}

func (gwr *GatewayAPIRouter) updateExistingRefs(canary *flaggerv1.Canary, pw, cw *int32) error {
	names := canaryBackendNamesOf(canary)

	canaryPins, primaryPins, err := gwr.pinMatches(canary)
	if err != nil {
		return err
	}

	// Read and compute everything first, so a misconfigured route is reported before any
	// route is changed.
	var updates []*v1.HTTPRoute
	for _, ref := range canary.Spec.Service.ExistingRefs {
		route, err := gwr.getRefRoute(canary, ref)
		if err != nil {
			return err
		}
		clone := route.DeepCopy()
		// The pin rules are rebuilt from the weighted rule on every pass: drop the old ones
		// first, so they are never taken for a weighted rule, and so they go away when the
		// pins are removed from the Canary.
		removePinRules(clone, names)
		_, matched := gwr.applyCanaryBackends(clone, names, canary.Spec.Service.Port, pw, cw)
		if !matched {
			return fmt.Errorf("HTTPRoute %s.%s has no backendRef to the Service %s, %s or %s",
				ref.Name, canary.Namespace, names.apex, names.primary, names.canary)
		}
		gwr.insertPinRules(clone, names, canary.Spec.Service.Port, canaryPins, primaryPins)
		if !equality.Semantic.DeepEqual(route.Spec.Rules, clone.Spec.Rules) {
			updates = append(updates, clone)
		}
	}

	for _, route := range updates {
		if _, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(route.Namespace).
			Update(context.TODO(), route, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("HTTPRoute %s.%s update error: %w", route.Name, route.Namespace, err)
		}
		gwr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("HTTPRoute %s.%s updated", route.Name, route.Namespace)
	}
	return nil
}

// getRoutesFromExistingRefs reads the weights from the first referenced route, after checking
// that the parents of every referenced route are ready.
func (gwr *GatewayAPIRouter) getRoutesFromExistingRefs(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, err error) {
	names := canaryBackendNamesOf(canary)
	found := false
	for i, ref := range canary.Spec.Service.ExistingRefs {
		var route *v1.HTTPRoute
		route, err = gwr.getRefRoute(canary, ref)
		if err != nil {
			return 0, 0, false, err
		}
		if err = httpRouteParentsReady(route); err != nil {
			return 0, 0, false, err
		}
		if i != 0 {
			continue
		}
		for _, rule := range route.Spec.Rules {
			var p, c *int32
			for _, backendRef := range rule.BackendRefs {
				switch string(backendRef.Name) {
				case names.primary:
					p = backendRef.Weight
				case names.canary:
					c = backendRef.Weight
				}
			}
			if p != nil && c != nil {
				primaryWeight, canaryWeight, found = int(*p), int(*c), true
				break
			}
		}
	}
	if !found {
		return 0, 0, false, fmt.Errorf("HTTPRoute %s.%s has no weighted %s/%s backendRefs yet",
			canary.Spec.Service.ExistingRefs[0].Name, canary.Namespace, names.primary, names.canary)
	}
	return primaryWeight, canaryWeight, false, nil
}

// finalizeExistingRefs puts the apex Service back in the referenced routes when the Canary is
// deleted with revertOnDeletion, because the primary and canary Services go away with it.
func (gwr *GatewayAPIRouter) finalizeExistingRefs(canary *flaggerv1.Canary) error {
	if !canary.Spec.RevertOnDeletion {
		return nil
	}
	names := canaryBackendNamesOf(canary)
	var failures []string
	for _, ref := range canary.Spec.Service.ExistingRefs {
		route, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(canary.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("get %s.%s: %v", ref.Name, canary.Namespace, err))
			continue
		}
		clone := route.DeepCopy()
		removePinRules(clone, names)
		gwr.revertCanaryBackends(clone, names, canary.Spec.Service.Port)
		if equality.Semantic.DeepEqual(route.Spec.Rules, clone.Spec.Rules) {
			continue
		}
		if _, err := gwr.gatewayAPIClient.GatewayapiV1().HTTPRoutes(canary.Namespace).
			Update(context.TODO(), clone, metav1.UpdateOptions{}); err != nil {
			failures = append(failures, fmt.Sprintf("update %s.%s: %v", ref.Name, canary.Namespace, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("reverting HTTPRoutes failed: %s", strings.Join(failures, "; "))
	}
	return nil
}
