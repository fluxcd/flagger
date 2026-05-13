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

package v2

import (
	"bytes"
	"encoding/gob"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// ApisixRoute is used to define the route rules and upstreams for Apache APISIX.
type ApisixRoute struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec              ApisixRouteSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status            ApisixStatus    `json:"status,omitempty" yaml:"status,omitempty"`
}

// ApisixStatus is the status report for Apisix ingress Resources
type ApisixStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// ApisixRouteSpec is the spec definition for ApisixRouteSpec.
type ApisixRouteSpec struct {
	HTTP   []ApisixRouteHTTP   `json:"http,omitempty" yaml:"http,omitempty"`
	Stream []ApisixRouteStream `json:"stream,omitempty" yaml:"stream,omitempty"`
}

// ApisixRouteHTTP represents a single route in for HTTP traffic.
type ApisixRouteHTTP struct {
	// The rule name, cannot be empty.
	Name string `json:"name" yaml:"name"`
	// Route priority, when multiple routes contains
	// same URI path (for path matching), route with
	// higher priority will take effect.
	Priority int                  `json:"priority,omitempty" yaml:"priority,omitempty"`
	Timeout  *UpstreamTimeout     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Match    ApisixRouteHTTPMatch `json:"match,omitempty" yaml:"match,omitempty"`
	// Backends represents potential backends to proxy after the route
	// rule matched. When number of backends are more than one, traffic-split
	// plugin in APISIX will be used to split traffic based on the backend weight.
	Backends         []ApisixRouteHTTPBackend   `json:"backends,omitempty" yaml:"backends,omitempty"`
	Websocket        bool                       `json:"websocket" yaml:"websocket"`
	PluginConfigName string                     `json:"plugin_config_name,omitempty" yaml:"plugin_config_name,omitempty"`
	Plugins          []ApisixRoutePlugin        `json:"plugins,omitempty" yaml:"plugins,omitempty"`
	Authentication   *ApisixRouteAuthentication `json:"authentication,omitempty" yaml:"authentication,omitempty"`
}

// UpstreamTimeout is settings for the read, send and connect to the upstream.
type UpstreamTimeout struct {
	Connect metav1.Duration `json:"connect,omitempty" yaml:"connect,omitempty"`
	Send    metav1.Duration `json:"send,omitempty" yaml:"send,omitempty"`
	Read    metav1.Duration `json:"read,omitempty" yaml:"read,omitempty"`
}

// ApisixRouteHTTPBackend represents a HTTP backend (a Kuberentes Service).
type ApisixRouteHTTPBackend struct {
	// The name (short) of the service, note cross namespace is forbidden,
	// so be sure the ApisixRoute and Service are in the same namespace.
	ServiceName string `json:"serviceName" yaml:"serviceName"`
	// The service port, could be the name or the port number.
	ServicePort intstr.IntOrString `json:"servicePort" yaml:"servicePort"`
	// The resolve granularity, can be "endpoints" or "service",
	// when set to "endpoints", the pod ips will be used; other
	// wise, the service ClusterIP or ExternalIP will be used,
	// default is endpoints.
	ResolveGranularity string `json:"resolveGranularity,omitempty" yaml:"resolveGranularity,omitempty"`
	// Weight of this backend.
	Weight *int `json:"weight" yaml:"weight"`
	// Subset specifies a subset for the target Service. The subset should be pre-defined
	// in ApisixUpstream about this service.
	Subset string `json:"subset,omitempty" yaml:"subset,omitempty"`
}

// ApisixRouteHTTPMatch represents the match condition for hitting this route.
type ApisixRouteHTTPMatch struct {
	// URI path predicates, at least one path should be
	// configured, path could be exact or prefix, for prefix path,
	// append "*" after it, for instance, "/foo*".
	Paths []string `json:"paths" yaml:"paths"`
	// HTTP request method predicates.
	Methods []string `json:"methods,omitempty" yaml:"methods,omitempty"`
	// HTTP Host predicates, host can be a wildcard domain or
	// an exact domain. For wildcard domain, only one generic
	// level is allowed, for instance, "*.foo.com" is valid but
	// "*.*.foo.com" is not.
	Hosts []string `json:"hosts,omitempty" yaml:"hosts,omitempty"`
	// Remote address predicates, items can be valid IPv4 address
	// or IPv6 address or CIDR.
	RemoteAddrs []string `json:"remoteAddrs,omitempty" yaml:"remoteAddrs,omitempty"`
	// NginxVars represents generic match predicates,
	// it uses Nginx variable systems, so any predicate
	// like headers, querystring and etc can be leveraged
	// here to match the route.
	// For instance, it can be:
	// nginxVars:
	//   - subject: "$remote_addr"
	//     op: in
	//     value:
	//       - "127.0.0.1"
	//       - "10.0.5.11"
	NginxVars []ApisixRouteHTTPMatchExpr `json:"exprs,omitempty" yaml:"exprs,omitempty"`
}

// ApisixRouteHTTPMatchExpr represents a binary route match expression .
type ApisixRouteHTTPMatchExpr struct {
	// Subject is the expression subject, it can
	// be any string composed by literals and nginx
	// vars.
	Subject ApisixRouteHTTPMatchExprSubject `json:"subject" yaml:"subject"`
	// Op is the operator.
	Op string `json:"op" yaml:"op"`
	// Set is an array type object of the expression.
	// It should be used when the Op is "in" or "not_in";
	Set []string `json:"set" yaml:"set"`
	// Value is the normal type object for the expression,
	// it should be used when the Op is not "in" and "not_in".
	// Set and Value are exclusive so only of them can be set
	// in the same time.
	Value *string `json:"value" yaml:"value"`
}

// ApisixRouteHTTPMatchExprSubject describes the route match expression subject.
type ApisixRouteHTTPMatchExprSubject struct {
	// The subject scope, can be:
	// ScopeQuery, ScopeHeader, ScopePath
	// when subject is ScopePath, Name field
	// will be ignored.
	Scope string `json:"scope" yaml:"scope"`
	// The name of subject.
	Name string `json:"name" yaml:"name"`
}

// ApisixRoutePlugin represents an APISIX plugin.
type ApisixRoutePlugin struct {
	// The plugin name.
	Name string `json:"name" yaml:"name"`
	// Whether this plugin is in use, default is true.
	Enable bool `json:"enable" yaml:"enable"`
	// Plugin configuration.
	Config ApisixRoutePluginConfig `json:"config" yaml:"config"`
}

// ApisixRoutePluginConfig is the configuration for
// any plugins.
type ApisixRoutePluginConfig map[string]interface{}

// ApisixRouteAuthentication is the authentication-related
// configuration in ApisixRoute.
type ApisixRouteAuthentication struct {
	Enable  bool                             `json:"enable" yaml:"enable"`
	Type    string                           `json:"type" yaml:"type"`
	KeyAuth ApisixRouteAuthenticationKeyAuth `json:"keyAuth,omitempty" yaml:"keyAuth,omitempty"`
	JwtAuth ApisixRouteAuthenticationJwtAuth `json:"jwtAuth,omitempty" yaml:"jwtAuth,omitempty"`
}

// ApisixRouteAuthenticationKeyAuth is the keyAuth-related
// configuration in ApisixRouteAuthentication.
type ApisixRouteAuthenticationKeyAuth struct {
	Header string `json:"header,omitempty" yaml:"header,omitempty"`
}

// ApisixRouteAuthenticationJwtAuth is the jwt auth related
// configuration in ApisixRouteAuthentication.
type ApisixRouteAuthenticationJwtAuth struct {
	Header string `json:"header,omitempty" yaml:"header,omitempty"`
	Query  string `json:"query,omitempty" yaml:"query,omitempty"`
	Cookie string `json:"cookie,omitempty" yaml:"cookie,omitempty"`
}

func (p ApisixRoutePluginConfig) DeepCopyInto(out *ApisixRoutePluginConfig) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	_ = enc.Encode(p)
	_ = dec.Decode(out)
}

func (p *ApisixRoutePluginConfig) DeepCopy() *ApisixRoutePluginConfig {
	if p == nil {
		return nil
	}
	out := new(ApisixRoutePluginConfig)
	p.DeepCopyInto(out)
	return out
}

// ApisixRouteStream is the configuration for level 4 route
type ApisixRouteStream struct {
	// The rule name, cannot be empty.
	Name     string                   `json:"name" yaml:"name"`
	Protocol string                   `json:"protocol" yaml:"protocol"`
	Match    ApisixRouteStreamMatch   `json:"match" yaml:"match"`
	Backend  ApisixRouteStreamBackend `json:"backend" yaml:"backend"`
	Plugins  []ApisixRoutePlugin      `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

// ApisixRouteStreamMatch represents the match conditions of stream route.
type ApisixRouteStreamMatch struct {
	// IngressPort represents the port listening on the Ingress proxy server.
	// It should be pre-defined as APISIX doesn't support dynamic listening.
	IngressPort int32 `json:"ingressPort" yaml:"ingressPort"`
}

// ApisixRouteStreamBackend represents a TCP backend (a Kubernetes Service).
type ApisixRouteStreamBackend struct {
	// The name (short) of the service, note cross namespace is forbidden,
	// so be sure the ApisixRoute and Service are in the same namespace.
	ServiceName string `json:"serviceName" yaml:"serviceName"`
	// The service port, could be the name or the port number.
	ServicePort intstr.IntOrString `json:"servicePort" yaml:"servicePort"`
	// The resolve granularity, can be "endpoints" or "service",
	// when set to "endpoints", the pod ips will be used; other
	// wise, the service ClusterIP or ExternalIP will be used,
	// default is endpoints.
	ResolveGranularity string `json:"resolveGranularity,omitempty" yaml:"resolveGranularity,omitempty"`
	// Subset specifies a subset for the target Service. The subset should be pre-defined
	// in ApisixUpstream about this service.
	Subset string `json:"subset,omitempty" yaml:"subset,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApisixRouteList contains a list of ApisixRoute.
type ApisixRouteList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata" yaml:"metadata"`
	Items           []ApisixRoute `json:"items,omitempty" yaml:"items,omitempty"`
}
