package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Upstream is a specification for a Gloo Upstream resource
type Upstream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec UpstreamSpec `json:"spec"`
}

type UpstreamSpec struct {
	Kube KubeUpstream `json:"kube,omitempty"`
}

type KubeUpstream struct {
	ServiceName      string            `json:"service_name,omitempty"`
	ServiceNamespace string            `json:"service_namespace,omitempty"`
	ServicePort      int32             `json:"service_port,omitempty"`
	Selector         map[string]string `json:"selector,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpstreamList is a list of Upstream resources
type UpstreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Upstream `json:"items"`
}
