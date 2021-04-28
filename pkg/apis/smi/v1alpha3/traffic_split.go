package v1alpha3

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TrafficSplit allows users to incrementally direct percentages of traffic
// between various services. It will be used by clients such as ingress
// controllers or service mesh sidecars to split the outgoing traffic to
// different destinations.
type TrafficSplit struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the traffic split.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec TrafficSplitSpec `json:"spec,omitempty"`
}

// TrafficSplitSpec is the specification for a TrafficSplit
type TrafficSplitSpec struct {
	// Service represents the apex service
	Service string `json:"service"`

	// Backends defines a list of Kubernetes services
	// used as the traffic split destination
	Backends []TrafficSplitBackend `json:"backends"`

	// Matches allows defining a list of HTTP route groups
	// that this traffic split object should match
	// +optional
	Matches []corev1.TypedLocalObjectReference `json:"matches,omitempty"`
}

// TrafficSplitBackend defines a backend
type TrafficSplitBackend struct {
	// Service is the name of a Kubernetes service
	Service string `json:"service"`

	// Weight defines the traffic split percentage
	Weight int `json:"weight"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type TrafficSplitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TrafficSplit `json:"items"`
}
