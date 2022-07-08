package v2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteTable is the Schema for the Gloo RouteTable resource
type RouteTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RouteTableSpec `json:"spec,omitempty"`
}

type RouteTableSpec struct {
	Hosts              []string              `json:"hosts,omitempty"`
	VirtualGateways    []*ObjectReference    `json:"virtualGateways,omitempty"`
	WorkloadSelectors  []*WorkloadSelector   `json:"workloadSelectors,omitempty"`
	DefaultDestination *DestinationReference `json:"defaultDestination,omitempty"`
	Http               []*HTTPRoute          `json:"http,omitempty"`
	Weight             int32                 `json:"weight,omitempty"`
}

type HTTPRoute struct {
	// unique name of the route (within the route table). used to identify the route for metrics
	Name string ` json:"name,omitempty"`
	// labels for the route. used to apply policies which implement routeSelectors.
	Labels map[string]string `json:"labels,omitempty"`
	// the set of request matchers which this route will match on. if none are specified, this route will match any HTTP traffic.
	Matchers []*HTTPRequestMatcher ` json:"matchers,omitempty"`

	Delegate *DelegateAction `json:"delegate,omitempty"`

	ForwardTo *ForwardToAction `json:"forwardTo,omitempty"`
}

type DelegateAction struct {
	// Delegate to the RouteTables that match the given selectors.
	// Selected route tables are ordered by creation time stamp in ascending order to guarantee consistent ordering.
	// Route tables will be selected from the pool of route tables defined within the current workspace, as well as any imported into the workspace.
	RouteTables []*ObjectSelector `json:"routeTables,omitempty"`
}

// ForwardToAction is an action to forward traffic to a list of destinations
type ForwardToAction struct {
	Destinations []*DestinationReference `json:"destinations,omitempty"`
	PathRewrite  string                  `json:"path_rewrite,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteTableList is a list of Upstream resources
type RouteTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RouteTable `json:"items"`
}

// ObjectReference is the Schema for the ObjectReference API
type ObjectReference struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

type ObjectSelector struct {
	Labels    map[string]string `json:"labels,omitempty"`
	Name      string            `json:"name,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
}

type WorkloadSelector struct {
	// Selector used to match Workload objects by their metadata.
	Selector *ObjectSelector `json:"selector,omitempty"`
	// The kind of workload being selected. Defaults to Kube.
	Kind WorkloadSelector_WorkloadKind `json:"kind,omitempty"`
	// The port to select on the selected workloads.
	// Only applies to policies which select specific workload ports, such as the WasmDeploymentPolicy.
	Port *PortSelector `json:"port,omitempty"`
}

type WorkloadSelector_WorkloadKind int32

type PortSelector struct {
	// the number of the port on the destination objects being targeted.
	Number uint32 `json:"number,omitempty"`

	// the name of the port on the destination objects being targeted.
	Name string `json:"name,omitempty"`
}

type DestinationKind int32

type DestinationReference struct {
	// reference to the destination object by its metadata
	Ref *ObjectReference `json:"ref,omitempty"`
	// the kind of destination being selected. defaults to Kubernetes Service.
	Kind DestinationKind `json:"kind,omitempty"`
	// the port on the destination object being targeted. required if the object provides more than one port.
	Port *PortSelector `json:"port,omitempty"`
	// select a subset of the destination's endpoints for routing based on their labels.
	Subset map[string]string `json:"subset,omitempty"`
	// Specify the proportion of traffic to be forwarded to this destination.
	// Weights across all of the `destinations` must sum to 100.
	// Weight is only relevant when used in the context of a route with multiple destinations.
	Weight uint32 `json:"weight,omitempty"`
}

type HTTPRequestMatcher struct {
	// Optional: The name assigned to a match. The match's name will be
	// concatenated with the parent route's name and will be logged in
	// the access logs for requests matching this route.
	Name string `json:"name,omitempty"`
	// Optional: Specify match criteria against the targeted path.
	Uri *StringMatch `json:"uri,omitempty"`
	// Optional: Specify a set of headers which requests must match in entirety (all headers must match).
	Headers []*HeaderMatcher `json:"headers,omitempty"`
	// Optional: Specify a set of URL query parameters which requests must match in entirety (all query params must match).
	QueryParameters []*HTTPRequestMatcher_QueryParameterMatcher `json:"query_parameters,omitempty"`
	// Optional: Specify an HTTP method to match against.
	Method string `json:"method,omitempty"`
}

type HTTPRequestMatcher_QueryParameterMatcher struct {
	// Specify the name of a key that must be present in the requested path's query string.
	Name string `json:"name,omitempty"`
	// Specify the value of the query parameter keyed on `name`.
	Value string `json:"value,omitempty"`
	// If true, treat `value` as a regular expression.
	Regex bool `json:"regex,omitempty"`
}

type StringMatch struct {
	// Exact string match.
	Exact string `json:"exact,omitempty"`
	// Prefix-based match.
	Prefix string `json:"prefix,omitempty"`
	// ECMAscript style regex-based match.
	Regex string `json:"regex,omitempty"`
	// Suffix-based match.@
	Suffix string `json:"suffix,omitempty"`

	//If true, indicates the exact/prefix/suffix matching should be case insensitive. This has no effect for the regex match.
	IgnoreCase bool `json:"ignore_case,omitempty"`
}

type HeaderMatcher struct {
	// Specify the name of the header in the request.
	Name string `json:"name,omitempty"`
	// Specify the value of the header. If the value is absent a request that
	// has the name header will match, regardless of the header’s value.
	Value string `json:"value,omitempty"`
	// Specify whether the header value should be treated as regex.
	Regex bool `json:"regex,omitempty"`
	//
	//If set to true, the result of the match will be inverted. Defaults to false.
	//
	//Examples:
	//
	//- name=foo, invert_match=true: matches if no header named `foo` is present
	//- name=foo, value=bar, invert_match=true: matches if no header named `foo` with value `bar` is present
	//- name=foo, value=``\d{3}``, regex=true, invert_match=true: matches if no header named `foo` with a value consisting of three integers is present.
	InvertMatch bool `json:"invert_match,omitempty"`
}
