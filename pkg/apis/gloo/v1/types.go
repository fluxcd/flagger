package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteTable is a specification for a Gloo RouteTable resource
type RouteTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RouteTableSpec `json:"spec"`
}

type RouteTableSpec struct {
	Routes []Route `json:"routes,omitempty"`
}

type Route struct {
	Matchers                []Matcher   `json:"matchers,omitempty"`
	Action                  RouteAction `json:"routeAction,omitempty"`
	InheritablePathMatchers bool        `json:"inheritablePathMatchers,omitempty"`
}

type Matcher struct {
	Headers                []HeaderMatcher         `json:"headers,omitempty"`
	QueryParameterMatchers []QueryParameterMatcher `json:"queryParameters,omitempty"`
	Methods                []string                `json:"methods,omitempty"`
}

type HeaderMatcher struct {
	Name        string `json:"name,omitempty"`
	Value       string `json:"value,omitempty"`
	Regex       bool   `json:"regex,omitempty"`
	InvertMatch bool   `json:"invertMatch,omitempty"`
}

type QueryParameterMatcher struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
	Regex bool   `json:"regex,omitempty"`
}

type RouteAction struct {
	Destination MultiDestination `json:"multi,omitempty"`
}

type MultiDestination struct {
	Destinations []WeightedDestination `json:"destinations,omitempty"`
}

// WeightedDestination attaches a weight to a single destination.
type WeightedDestination struct {
	Destination Destination `json:"destination,omitempty"`
	// Weight must be greater than zero
	// Routing to each destination will be balanced by the ratio of the destination's weight to the total weight on a route
	Weight uint32 `json:"weight,omitempty"`
}

// Destinations define routable destinations for proxied requests
type Destination struct {
	Upstream ResourceRef `json:"upstream"`
}

// ResourceRef references resources across namespaces
type ResourceRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteTableList is a list of RouteTable resources
type RouteTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RouteTable `json:"items"`
}
