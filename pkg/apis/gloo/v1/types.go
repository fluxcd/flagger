package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpstreamGroup is a specification for a Gloo UpstreamGroup resource
type UpstreamGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec UpstreamGroupSpec `json:"spec"`
}

type UpstreamGroupSpec struct {
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

// UpstreamGroupList is a list of UpstreamGroup resources
type UpstreamGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []UpstreamGroup `json:"items"`
}
