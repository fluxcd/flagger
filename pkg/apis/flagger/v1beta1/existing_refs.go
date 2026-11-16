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

package v1beta1

import (
	"fmt"
	"sort"
	"strings"

	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
)

// HasExistingRefs returns true when the Canary asks Flagger to manage the weights of
// existing HTTPRoutes instead of generating one.
func (c *Canary) HasExistingRefs() bool {
	return len(c.Spec.Service.ExistingRefs) > 0
}

// HasExistingRefPins returns true when the Canary asks Flagger to add, next to the weighted
// rule of every referenced route, one rule that pins a request to the canary
// (spec.analysis.match) and one that pins it to the primary (spec.analysis.primaryMatch).
func (c *Canary) HasExistingRefPins() bool {
	analysis := c.GetAnalysis()
	return c.HasExistingRefs() && analysis != nil && len(analysis.Match) > 0 && len(analysis.PrimaryMatch) > 0
}

// ValidateExistingRefs checks that spec.service.existingRefs is not combined with
// anything that only makes sense for a generated HTTPRoute. It returns nil when no
// refs are set.
func (c *Canary) ValidateExistingRefs() error {
	if !c.HasExistingRefs() {
		return nil
	}

	svc := c.Spec.Service
	seen := make(map[string]struct{}, len(svc.ExistingRefs))
	for i, ref := range svc.ExistingRefs {
		if err := c.validateExistingRef(i, ref); err != nil {
			return err
		}
		key := ref.Kind + "/" + ref.Name
		if _, ok := seen[key]; ok {
			return fmt.Errorf("spec.service.existingRefs lists %s %q more than once", ref.Kind, ref.Name)
		}
		seen[key] = struct{}{}
	}

	var conflicts []string
	add := func(set bool, field string) {
		if set {
			conflicts = append(conflicts, field)
		}
	}
	add(len(svc.GatewayRefs) > 0, "spec.service.gatewayRefs")
	add(len(svc.Hosts) > 0, "spec.service.hosts")
	add(len(svc.Match) > 0, "spec.service.match")
	add(svc.Rewrite != nil, "spec.service.rewrite")
	add(svc.Headers != nil, "spec.service.headers")
	add(svc.Retries != nil, "spec.service.retries")
	add(len(svc.Mirror) > 0, "spec.service.mirror")
	add(svc.CorsPolicy != nil, "spec.service.corsPolicy")
	add(svc.Timeout != "", "spec.service.timeout")
	add(svc.BackendTimeout != "", "spec.service.backendTimeout")

	if analysis := c.GetAnalysis(); analysis != nil {
		add(analysis.SessionAffinity != nil, "spec.analysis.sessionAffinity")
		add(analysis.Mirror, "spec.analysis.mirror")
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("spec.service.existingRefs cannot be combined with %s: the referenced HTTPRoutes keep their own configuration",
			strings.Join(conflicts, ", "))
	}
	return c.validateExistingRefPins()
}

// validateExistingRef checks one entry of spec.service.existingRefs.
func (c *Canary) validateExistingRef(i int, ref ExistingReference) error {
	field := fmt.Sprintf("spec.service.existingRefs[%d]", i)
	if ref.Kind == "" {
		return fmt.Errorf("%s.kind must be set, supported: %s", field, ExistingRefKindHTTPRoute)
	}
	if ref.Kind != ExistingRefKindHTTPRoute {
		return fmt.Errorf("%s.kind %q is not supported, supported: %s", field, ref.Kind, ExistingRefKindHTTPRoute)
	}
	if ref.Group != "" && ref.Group != ExistingRefGroupGatewayAPI {
		return fmt.Errorf("%s.group %q is not the group of %s, use %q or leave it empty",
			field, ref.Group, ref.Kind, ExistingRefGroupGatewayAPI)
	}
	if ref.Name == "" {
		return fmt.Errorf("%s.name must not be empty", field)
	}
	return nil
}

// validateExistingRefPins checks spec.analysis.match and spec.analysis.primaryMatch, the only
// parts of the analysis that are allowed with spec.service.existingRefs. They are set together
// and only use headers and query parameters: the path and the method come from the referenced
// route, which the pin rules copy.
func (c *Canary) validateExistingRefPins() error {
	analysis := c.GetAnalysis()
	if analysis == nil || (len(analysis.Match) == 0 && len(analysis.PrimaryMatch) == 0) {
		return nil
	}
	if len(analysis.Match) == 0 || len(analysis.PrimaryMatch) == 0 {
		return fmt.Errorf("spec.analysis.match (pins to the canary) and spec.analysis.primaryMatch (pins to the primary) " +
			"must be set together with spec.service.existingRefs")
	}
	if analysis.Iterations > 0 {
		return fmt.Errorf("spec.analysis.iterations (A/B testing, blue/green) cannot be combined with spec.service.existingRefs: " +
			"spec.analysis.match and spec.analysis.primaryMatch pin traffic on top of the weighted split")
	}

	canaryPins := make(map[string]struct{}, len(analysis.Match))
	for i, m := range analysis.Match {
		key, err := pinMatchKey(m)
		if err != nil {
			return fmt.Errorf("spec.analysis.match[%d]: %w", i, err)
		}
		canaryPins[key] = struct{}{}
	}
	for i, m := range analysis.PrimaryMatch {
		key, err := pinMatchKey(m)
		if err != nil {
			return fmt.Errorf("spec.analysis.primaryMatch[%d]: %w", i, err)
		}
		if _, ok := canaryPins[key]; ok {
			return fmt.Errorf("spec.analysis.primaryMatch[%d] is identical to an entry of spec.analysis.match: "+
				"one request would match both versions", i)
		}
	}
	return nil
}

// pinMatchKey validates a pin and returns a canonical key for it, so two pins can be compared.
func pinMatchKey(m istiov1beta1.HTTPMatchRequest) (string, error) {
	if m.Uri != nil || m.Scheme != nil || m.Method != nil || m.Authority != nil || m.Port != 0 ||
		len(m.SourceLabels) > 0 || len(m.Gateways) > 0 || m.IgnoreUriCase || len(m.WithoutHeaders) > 0 ||
		m.SourceNamespace != "" {
		return "", fmt.Errorf("only headers and queryParams are supported, the path and the method come from the referenced HTTPRoute")
	}
	if len(m.Headers) == 0 && len(m.QueryParams) == 0 {
		return "", fmt.Errorf("must set at least one header or query parameter")
	}

	var parts []string
	collect := func(kind string, set map[string]istiov1alpha1.StringMatch) error {
		for name, v := range set {
			if kind == "header" {
				name = strings.ToLower(name) // header names are case insensitive
			}
			switch {
			case v.Exact != "":
				parts = append(parts, kind+":"+name+"=exact:"+v.Exact)
			case v.Regex != "":
				parts = append(parts, kind+":"+name+"=regex:"+v.Regex)
			default:
				return fmt.Errorf("%s %q must set exact or regex", kind, name)
			}
		}
		return nil
	}
	if err := collect("header", m.Headers); err != nil {
		return "", err
	}
	if err := collect("query", m.QueryParams); err != nil {
		return "", err
	}
	sort.Strings(parts)
	return strings.Join(parts, "&"), nil
}
