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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1beta1"
	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
)

func httpRouteRef(name string) ExistingReference {
	return ExistingReference{Kind: ExistingRefKindHTTPRoute, Name: name}
}

func canaryWithRefs(mutate func(*Canary)) *Canary {
	c := &Canary{
		Spec: CanarySpec{
			Service: CanaryService{
				Port:         80,
				ExistingRefs: []ExistingReference{httpRouteRef("ingress-route"), httpRouteRef("svc-route")},
			},
			Analysis: &CanaryAnalysis{Threshold: 5, StepWeight: 10, MaxWeight: 50},
		},
	}
	if mutate != nil {
		mutate(c)
	}
	return c
}

func pinHeaderRequest(value string) []istiov1beta1.HTTPMatchRequest {
	return []istiov1beta1.HTTPMatchRequest{
		{Headers: map[string]istiov1alpha1.StringMatch{"x-ice-podinfo-version": {Exact: value}}},
	}
}

func TestCanary_HasExistingRefs(t *testing.T) {
	assert.False(t, (&Canary{}).HasExistingRefs())
	assert.True(t, canaryWithRefs(nil).HasExistingRefs())
}

func TestCanary_HasExistingRefPins(t *testing.T) {
	assert.False(t, canaryWithRefs(nil).HasExistingRefPins())
	assert.False(t, canaryWithRefs(func(c *Canary) { c.Spec.Analysis.Match = pinHeaderRequest("canary") }).HasExistingRefPins())
	assert.True(t, canaryWithRefs(func(c *Canary) {
		c.Spec.Analysis.Match = pinHeaderRequest("canary")
		c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
	}).HasExistingRefPins())

	// pins only make sense with existingRefs
	assert.False(t, (&Canary{Spec: CanarySpec{Analysis: &CanaryAnalysis{
		Match: pinHeaderRequest("canary"), PrimaryMatch: pinHeaderRequest("primary"),
	}}}).HasExistingRefPins())
}

func TestCanary_ValidateExistingRefs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Canary)
		wantErr string
	}{
		{name: "refs only", mutate: nil},
		{name: "no refs is always valid", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = nil
			c.Spec.Service.GatewayRefs = []v1beta1.ParentReference{{Name: "gw"}}
		}},
		{name: "empty name", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = []ExistingReference{{Kind: ExistingRefKindHTTPRoute, Name: ""}}
		}, wantErr: "existingRefs[0].name must not be empty"},
		{name: "duplicate", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = []ExistingReference{httpRouteRef("a"), httpRouteRef("a")}
		}, wantErr: `lists HTTPRoute "a" more than once`},
		{name: "kind and group of an HTTPRoute", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = []ExistingReference{
				{Group: ExistingRefGroupGatewayAPI, Kind: ExistingRefKindHTTPRoute, Name: "a"},
			}
		}},
		{name: "kind missing", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = []ExistingReference{{Name: "a"}}
		}, wantErr: "existingRefs[0].kind must be set, supported: HTTPRoute"},
		{name: "kind not supported", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = []ExistingReference{{Kind: "Ingress", Name: "a"}}
		}, wantErr: `existingRefs[0].kind "Ingress" is not supported`},
		{name: "group of another kind", mutate: func(c *Canary) {
			c.Spec.Service.ExistingRefs = []ExistingReference{{Group: "networking.k8s.io", Kind: ExistingRefKindHTTPRoute, Name: "a"}}
		}, wantErr: `existingRefs[0].group "networking.k8s.io"`},
		{name: "gatewayRefs", mutate: func(c *Canary) {
			c.Spec.Service.GatewayRefs = []v1beta1.ParentReference{{Name: "gw"}}
		}, wantErr: "spec.service.gatewayRefs"},
		{name: "hosts", mutate: func(c *Canary) { c.Spec.Service.Hosts = []string{"a.example.com"} }, wantErr: "spec.service.hosts"},
		{name: "match", mutate: func(c *Canary) {
			c.Spec.Service.Match = []istiov1beta1.HTTPMatchRequest{{Uri: &istiov1alpha1.StringMatch{Prefix: "/x"}}}
		}, wantErr: "spec.service.match"},
		{name: "rewrite", mutate: func(c *Canary) { c.Spec.Service.Rewrite = &HTTPRewrite{Uri: "/"} }, wantErr: "spec.service.rewrite"},
		{name: "headers", mutate: func(c *Canary) { c.Spec.Service.Headers = &istiov1beta1.Headers{} }, wantErr: "spec.service.headers"},
		{name: "retries", mutate: func(c *Canary) { c.Spec.Service.Retries = &istiov1beta1.HTTPRetry{Attempts: 3} }, wantErr: "spec.service.retries"},
		{name: "timeout", mutate: func(c *Canary) { c.Spec.Service.Timeout = "10s" }, wantErr: "spec.service.timeout"},
		{name: "backendTimeout", mutate: func(c *Canary) { c.Spec.Service.BackendTimeout = "5s" }, wantErr: "spec.service.backendTimeout"},
		{name: "match and primaryMatch", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = pinHeaderRequest("canary")
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
		}},
		{name: "match and primaryMatch on query parameters", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = []istiov1beta1.HTTPMatchRequest{{QueryParams: map[string]istiov1alpha1.StringMatch{"v": {Exact: "canary"}}}}
			c.Spec.Analysis.PrimaryMatch = []istiov1beta1.HTTPMatchRequest{{QueryParams: map[string]istiov1alpha1.StringMatch{"v": {Regex: "prim.*"}}}}
		}},
		{name: "match without primaryMatch", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = pinHeaderRequest("canary")
		}, wantErr: "spec.analysis.match (pins to the canary) and spec.analysis.primaryMatch (pins to the primary) must be set together"},
		{name: "primaryMatch without match", mutate: func(c *Canary) {
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
		}, wantErr: "must be set together"},
		{name: "pins with iterations (A/B, blue/green)", mutate: func(c *Canary) {
			c.Spec.Analysis.Iterations = 10
			c.Spec.Analysis.Match = pinHeaderRequest("canary")
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
		}, wantErr: "spec.analysis.iterations"},
		{name: "pin with a path", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = []istiov1beta1.HTTPMatchRequest{{Uri: &istiov1alpha1.StringMatch{Prefix: "/x"}}}
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
		}, wantErr: "spec.analysis.match[0]: only headers and queryParams are supported"},
		{name: "primary pin with a method", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = pinHeaderRequest("canary")
			c.Spec.Analysis.PrimaryMatch = []istiov1beta1.HTTPMatchRequest{{Method: &istiov1alpha1.StringMatch{Exact: "GET"}}}
		}, wantErr: "spec.analysis.primaryMatch[0]: only headers and queryParams are supported"},
		{name: "empty pin", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = []istiov1beta1.HTTPMatchRequest{{}}
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
		}, wantErr: "must set at least one header or query parameter"},
		{name: "header without exact or regex", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = []istiov1beta1.HTTPMatchRequest{{Headers: map[string]istiov1alpha1.StringMatch{"x": {Prefix: "p"}}}}
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("primary")
		}, wantErr: `header "x" must set exact or regex`},
		{name: "the same pin for both versions", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = pinHeaderRequest("same")
			c.Spec.Analysis.PrimaryMatch = pinHeaderRequest("same")
		}, wantErr: "identical to an entry of spec.analysis.match"},
		{name: "the same pin, header names differ in case", mutate: func(c *Canary) {
			c.Spec.Analysis.Match = []istiov1beta1.HTTPMatchRequest{{Headers: map[string]istiov1alpha1.StringMatch{"X-Ice": {Exact: "v"}}}}
			c.Spec.Analysis.PrimaryMatch = []istiov1beta1.HTTPMatchRequest{{Headers: map[string]istiov1alpha1.StringMatch{"x-ice": {Exact: "v"}}}}
		}, wantErr: "identical to an entry of spec.analysis.match"},
		{name: "session affinity", mutate: func(c *Canary) {
			c.Spec.Analysis.SessionAffinity = &SessionAffinity{CookieName: "c"}
		}, wantErr: "spec.analysis.sessionAffinity"},
		{name: "mirroring", mutate: func(c *Canary) { c.Spec.Analysis.Mirror = true }, wantErr: "spec.analysis.mirror"},
		{name: "several conflicts are all named", mutate: func(c *Canary) {
			c.Spec.Service.Hosts = []string{"a"}
			c.Spec.Service.Timeout = "1s"
		}, wantErr: "spec.service.hosts, spec.service.timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := canaryWithRefs(tt.mutate).ValidateExistingRefs()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
