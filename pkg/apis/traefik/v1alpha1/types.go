package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TraefikService is the specification for a service (that an IngressRoute refers
// to) that is usually not a terminal service (i.e. not a pod of servers), as
// opposed to a Kubernetes Service. That is to say, it usually refers to other
// (children) services, which themselves can be TraefikServices or Services.
type TraefikService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec ServiceSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TraefikServiceList is a list of TraefikService resources.
type TraefikServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []TraefikService `json:"items"`
}

// ServiceSpec defines whether a TraefikService is a load-balancer of services or a
// mirroring service.
type ServiceSpec struct {
	Weighted *WeightedRoundRobin `json:"weighted,omitempty"`
}

// WeightedRoundRobin defines a load-balancer of services.
type WeightedRoundRobin struct {
	Services []Service `json:"services,omitempty"`
}

type Service struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Port      int32  `json:"port"`
	Weight    uint   `json:"weight,omitempty"`
}
