package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
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
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the desired behavior of the traffic split.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	Spec TrafficSplitSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Most recently observed status of the pod.
	// This data may not be up to date.
	// Populated by the system.
	// Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status
	// +optional
	//Status Status `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// TrafficSplitSpec is the specification for a TrafficSplit
type TrafficSplitSpec struct {
	Service  string                `json:"service,omitempty"`
	Backends []TrafficSplitBackend `json:"backends,omitempty"`
}

// TrafficSplitBackend defines a backend
type TrafficSplitBackend struct {
	Service string             `json:"service,omitempty"`
	Weight  *resource.Quantity `json:"weight,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type TrafficSplitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TrafficSplit `json:"items"`
}
