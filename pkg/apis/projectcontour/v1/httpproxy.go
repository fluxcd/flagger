// Copyright © 2019 VMware
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HTTPProxySpec defines the spec of the CRD.
type HTTPProxySpec struct {
	// Virtualhost appears at most once. If it is present, the object is considered
	// to be a "root".
	// +optional
	VirtualHost *VirtualHost `json:"virtualhost,omitempty"`
	// Routes are the ingress routes. If TCPProxy is present, Routes is ignored.
	//  +optional
	Routes []Route `json:"routes,omitempty"`
	// TCPProxy holds TCP proxy information.
	// +optional
	TCPProxy *TCPProxy `json:"tcpproxy,omitempty"`
	// Includes allow for specific routing configuration to be appended to another HTTPProxy in another namespace.
	// +optional
	Includes []Include `json:"includes,omitempty"`
}

// Include describes a set of policies that can be applied to an HTTPProxy in a namespace.
type Include struct {
	// Name of the HTTPProxy
	Name string `json:"name"`
	// Namespace of the HTTPProxy to include. Defaults to the current namespace if not supplied.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Conditions are a set of routing properties that is applied to an HTTPProxy in a namespace.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// Condition are policies that are applied on top of HTTPProxies.
// One of Prefix or Header must be provided.
type Condition struct {
	// Prefix defines a prefix match for a request.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// Header specifies the header condition to match.
	// +optional
	Header *HeaderCondition `json:"header,omitempty"`
}

// HeaderCondition specifies the header condition to match.
// Name is required. Only one of Present or Contains must
// be provided.
type HeaderCondition struct {

	// Name is the name of the header to match on. Name is required.
	// Header names are case insensitive.
	Name string `json:"name"`

	// Present is true if the Header is present in the request.
	// +optional
	Present bool `json:"present,omitempty"`

	// Contains is true if the Header containing this string is present
	// in the request.
	// +optional
	Contains string `json:"contains,omitempty"`

	// NotContains is true if the Header containing this string is not present
	// in the request.
	// +optional
	NotContains string `json:"notcontains,omitempty"`

	// Exact is true if the Header containing this string matches exactly
	// in the request.
	// +optional
	Exact string `json:"exact,omitempty"`

	// NotExact is true if the Header containing this string doesn't match exactly
	// in the request.
	// +optional
	NotExact string `json:"notexact,omitempty"`
}

// VirtualHost appears at most once. If it is present, the object is considered
// to be a "root".
type VirtualHost struct {
	// The fully qualified domain name of the root of the ingress tree
	// all leaves of the DAG rooted at this object relate to the fqdn
	Fqdn string `json:"fqdn"`
	// If present describes tls properties. The CNI names that will be matched on
	// are described in fqdn, the tls.secretName secret must contain a
	// matching certificate
	// +optional
	TLS *TLS `json:"tls,omitempty"`
}

// TLS describes tls properties. The CNI names that will be matched on
// are described in fqdn, the tls.secretName secret must contain a
// matching certificate unless tls.passthrough is set to true.
type TLS struct {
	// required, the name of a secret in the current namespace
	SecretName string `json:"secretName,omitempty"`
	// Minimum TLS version this vhost should negotiate
	// +optional
	MinimumProtocolVersion string `json:"minimumProtocolVersion,omitempty"`
	// If Passthrough is set to true, the SecretName will be ignored
	// and the encrypted handshake will be passed through to the
	// backing cluster.
	// +optional
	Passthrough bool `json:"passthrough,omitempty"`
}

// Route contains the set of routes for a virtual host.
type Route struct {
	// Conditions are a set of routing properties that is applied to an HTTPProxy in a namespace.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// Services are the services to proxy traffic.
	Services []Service `json:"services,omitempty"`
	// Enables websocket support for the route.
	// +optional
	EnableWebsockets bool `json:"enableWebsockets,omitempty"`
	// Allow this path to respond to insecure requests over HTTP which are normally
	// not permitted when a `virtualhost.tls` block is present.
	// +optional
	PermitInsecure bool `json:"permitInsecure,omitempty"`
	// The timeout policy for this route.
	// +optional
	TimeoutPolicy *TimeoutPolicy `json:"timeoutPolicy,omitempty"`
	// The retry policy for this route.
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`
	// The health check policy for this route.
	// +optional
	HealthCheckPolicy *HTTPHealthCheckPolicy `json:"healthCheckPolicy,omitempty"`
	// The load balancing policy for this route.
	// +optional
	LoadBalancerPolicy *LoadBalancerPolicy `json:"loadBalancerPolicy,omitempty"`
	// The policy for rewriting the path of the request URL
	// after the request has been routed to a Service.
	//
	// +optional
	PathRewritePolicy *PathRewritePolicy `json:"pathRewritePolicy,omitempty"`
	// The policy for managing request headers during proxying
	// +optional
	RequestHeadersPolicy *HeadersPolicy `json:"requestHeadersPolicy,omitempty"`
	// The policy for managing response headers during proxying
	// +optional
	ResponseHeadersPolicy *HeadersPolicy `json:"responseHeadersPolicy,omitempty"`
}

func (r *Route) GetPrefixReplacements() []ReplacePrefix {
	if r.PathRewritePolicy != nil {
		return r.PathRewritePolicy.ReplacePrefix
	}
	return nil
}

// TCPProxy contains the set of services to proxy TCP connections.
type TCPProxy struct {
	// The load balancing policy for the backend services.
	// +optional
	LoadBalancerPolicy *LoadBalancerPolicy `json:"loadBalancerPolicy,omitempty"`
	// Services are the services to proxy traffic
	Services []Service `json:"services,omitempty"`

	// Include specifies that this tcpproxy should be delegated to another HTTPProxy.
	// +optional
	Include *TCPProxyInclude `json:"includes,omitempty"`
}

// TCPProxyInclude describes a target HTTPProxy document which contains the TCPProxy details.
type TCPProxyInclude struct {
	// Name of the child HTTPProxy
	Name string `json:"name"`
	// Namespace of the HTTPProxy to include. Defaults to the current namespace if not supplied.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// Service defines an Kubernetes Service to proxy traffic.
type Service struct {
	// Name is the name of Kubernetes service to proxy traffic.
	// Names defined here will be used to look up corresponding endpoints which contain the ips to route.
	Name string `json:"name"`
	// Port (defined as Integer) to proxy traffic to since a service can have multiple defined.
	Port int `json:"port"`
	// Protocol may be used to specify (or override) the protocol used to reach this Service.
	// Values may be tls, h2, h2c.  It ommitted protocol-selection falls back on Service annotations.
	// +optional
	Protocol *string `json:"protocol,omitempty"`
	// Weight defines percentage of traffic to balance traffic
	// +optional
	Weight uint32 `json:"weight,omitempty"`
	// UpstreamValidation defines how to verify the backend service's certificate
	// +optional
	UpstreamValidation *UpstreamValidation `json:"validation,omitempty"`
	// If Mirror is true the Service will receive a read only mirror of the traffic for this route.
	Mirror bool `json:"mirror,omitempty"`
	// The policy for managing request headers during proxying
	// +optional
	RequestHeadersPolicy *HeadersPolicy `json:"requestHeadersPolicy,omitempty"`
	// The policy for managing response headers during proxying
	// +optional
	ResponseHeadersPolicy *HeadersPolicy `json:"responseHeadersPolicy,omitempty"`
}

// HTTPHealthCheckPolicy defines health checks on the upstream service.
type HTTPHealthCheckPolicy struct {
	// HTTP endpoint used to perform health checks on upstream service
	Path string `json:"path"`
	// The value of the host header in the HTTP health check request.
	// If left empty (default value), the name "contour-envoy-healthcheck"
	// will be used.
	Host string `json:"host,omitempty"`
	// The interval (seconds) between health checks
	// +optional
	IntervalSeconds int64 `json:"intervalSeconds"`
	// The time to wait (seconds) for a health check response
	// +optional
	TimeoutSeconds int64 `json:"timeoutSeconds"`
	// The number of unhealthy health checks required before a host is marked unhealthy
	// +optional
	UnhealthyThresholdCount uint32 `json:"unhealthyThresholdCount"`
	// The number of healthy health checks required before a host is marked healthy
	// +optional
	HealthyThresholdCount uint32 `json:"healthyThresholdCount"`
}

// TimeoutPolicy defines the attributes associated with timeout.
type TimeoutPolicy struct {
	// TimeoutPolicy durations are expressed as per the format specified in the ParseDuration documentation: https://godoc.org/time#ParseDuration
	// Example input values: "300ms", "5s", "1m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	// The string 'infinity' is also a valid input and specifies no timeout.

	// Timeout for receiving a response from the server after processing a request from client.
	// If not supplied the timeout duration is undefined.
	// +optional
	Response string `json:"response,omitempty"`

	// Timeout after which if there are no active requests for this route, the connection between
	// Envoy and the backend will be closed. If not specified, there is no per-route idle timeout.
	// +optional
	Idle string `json:"idle,omitempty"`
}

// RetryPolicy defines the attributes associated with retrying policy.
type RetryPolicy struct {
	// NumRetries is maximum allowed number of retries.
	// If not supplied, the number of retries is one.
	// +optional
	NumRetries uint32 `json:"count"`
	// PerTryTimeout specifies the timeout per retry attempt.
	// Ignored if NumRetries is not supplied.
	PerTryTimeout string `json:"perTryTimeout,omitempty"`
}

// ReplacePrefix describes a path prefix replacement.
type ReplacePrefix struct {
	// Prefix specifies the URL path prefix to be replaced.
	//
	// If Prefix is specified, it must exactly match the Condition
	// prefix that is rendered by the chain of including HTTPProxies
	// and only that path prefix will be replaced by Replacement.
	// This allows HTTPProxies that are included through multiple
	// roots to only replace specific path prefixes, leaving others
	// unmodified.
	//
	// If Prefix is not specified, all routing prefixes rendered
	// by the include chain will be replaced.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	Prefix string `json:"prefix,omitempty"`

	// Replacement is the string that the routing path prefix
	// will be replaced with. This must not be empty.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Replacement string `json:"replacement"`
}

// PathRewritePolicy specifies how a request URL path should be
// rewritten. This rewriting takes place after a request is routed
// and has no subsequent effects on the proxy's routing decision.
// No HTTP headers or body content is rewritten.
//
// Exactly one field in this struct may be specified.
type PathRewritePolicy struct {
	// ReplacePrefix describes how the path prefix should be replaced.
	// +optional
	ReplacePrefix []ReplacePrefix `json:"replacePrefix,omitempty"`
}

// LoadBalancerPolicy defines the load balancing policy.
type LoadBalancerPolicy struct {
	Strategy string `json:"strategy,omitempty"`
}

// HeadersPolicy defines how headers are managed during forwarding
type HeadersPolicy struct {
	// Set specifies a list of HTTP header values that will be set in the HTTP header
	// +optional
	Set []HeaderValue `json:"set,omitempty"`
	// Remove specifies a list of HTTP header names to remove
	// +optional
	Remove []string `json:"remove,omitempty"`
}

// HeaderValue represents a header name/value pair
type HeaderValue struct {
	// Name represents a key of a header
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Value represents the value of a header specified by a key
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}

// UpstreamValidation defines how to verify the backend service's certificate
type UpstreamValidation struct {
	// Name of the Kubernetes secret be used to validate the certificate presented by the backend
	CACertificate string `json:"caSecret"`
	// Key which is expected to be present in the 'subjectAltName' of the presented certificate
	SubjectName string `json:"subjectName"`
}

// Status reports the current state of the HTTPProxy.
type Status struct {
	// +optional
	CurrentStatus string `json:"currentStatus,omitempty"`
	// +optional
	Description string `json:"description,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HTTPProxy is an Ingress CRD specification
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="FQDN",type="string",JSONPath=".spec.virtualhost.fqdn",description="Fully qualified domain name"
// +kubebuilder:printcolumn:name="TLS Secret",type="string",JSONPath=".spec.virtualhost.tls.secretName",description="Secret with TLS credentials"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.currentStatus",description="The current status of the HTTPProxy"
// +kubebuilder:printcolumn:name="Status Description",type="string",JSONPath=".status.description",description="Description of the current status"
// +kubebuilder:resource:scope=Namespaced,path=httpproxies,shortName=proxy;proxies,singular=httpproxy
type HTTPProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec HTTPProxySpec `json:"spec"`
	// +optional
	Status Status `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HTTPProxyList is a list of HTTPProxies.
type HTTPProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []HTTPProxy `json:"items"`
}
