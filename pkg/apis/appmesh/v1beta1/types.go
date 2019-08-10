package v1beta1

import (
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// App Mesh Custom Resource API types.
// This API follows the conventions described in
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Mesh is a specification for a Mesh resource
type Mesh struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec MeshSpec `json:"spec,omitempty"`
	// +optional
	Status MeshStatus `json:"status,omitempty"`
}

type MeshServiceDiscoveryType string

const (
	Dns MeshServiceDiscoveryType = "Dns"
)

// MeshSpec is the spec for a Mesh resource
type MeshSpec struct {
	// +optional
	ServiceDiscoveryType *MeshServiceDiscoveryType `json:"serviceDiscoveryType,omitempty"`
}

// MeshStatus is the status for a Mesh resource
type MeshStatus struct {
	// MeshArn is the AppMesh Mesh object's Amazon Resource Name
	// +optional
	MeshArn    *string         `json:"meshArn,omitempty"`
	Conditions []MeshCondition `json:"meshCondition"`
}

type MeshConditionType string

const (
	// MeshActive is Active when the Appmesh Mesh has been created or found via the API
	MeshActive MeshConditionType = "MeshActive"
)

type MeshCondition struct {
	// Type of mesh condition.
	Type MeshConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status api.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason *string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message *string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MeshList is a list of Mesh resources
type MeshList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Mesh `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualService is a specification for a VirtualService resource
type VirtualService struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec VirtualServiceSpec `json:"spec,omitempty"`
	// +optional
	Status VirtualServiceStatus `json:"status,omitempty"`
}

// VirtualServiceSpec is the spec for a VirtualService resource
type VirtualServiceSpec struct {
	MeshName string `json:"meshName"`
	// +optional
	VirtualRouter *VirtualRouter `json:"virtualRouter,omitempty"`
	// +optional
	Routes []Route `json:"routes,omitempty"`
}

// VirtualRouter is the spec for a VirtualRouter resource
type VirtualRouter struct {
	Name      string     `json:"name"`
	Listeners []Listener `json:"listeners,omitempty"`
}

type Route struct {
	Name string `json:"name"`
	// +optional
	Http *HttpRoute `json:"http,omitempty"`
	// +optional
	Tcp *TcpRoute `json:"tcp,omitempty"`
}

type HttpRoute struct {
	Match  HttpRouteMatch  `json:"match"`
	Action HttpRouteAction `json:"action"`
}

type HttpRouteMatch struct {
	Prefix string `json:"prefix"`
}

type HttpRouteAction struct {
	WeightedTargets []WeightedTarget `json:"weightedTargets"`
}

type TcpRoute struct {
	Action TcpRouteAction `json:"action"`
}

type TcpRouteAction struct {
	WeightedTargets []WeightedTarget `json:"weightedTargets"`
}

type WeightedTarget struct {
	VirtualNodeName string `json:"virtualNodeName"`
	Weight          int64  `json:"weight"`
}

// VirtualServiceStatus is the status for a VirtualService resource
type VirtualServiceStatus struct {
	// VirtualServiceArn is the AppMesh VirtualService object's Amazon Resource Name
	// +optional
	VirtualServiceArn *string `json:"virtualServiceArn,omitempty"`
	// VirtualRouterArn is the AppMesh VirtualRouter object's Amazon Resource Name
	// +optional
	VirtualRouterArn *string `json:"virtualRouterArn,omitempty"`
	// RouteArns is a list of AppMesh Route objects' Amazon Resource Names
	// +optional
	RouteArns  []string                  `json:"routeArns,omitempty"`
	Conditions []VirtualServiceCondition `json:"conditions"`
}

type VirtualServiceConditionType string

const (
	// VirtualServiceActive is Active when the Appmesh Service has been created or found via the API
	VirtualServiceActive                VirtualServiceConditionType = "VirtualServiceActive"
	VirtualRouterActive                 VirtualServiceConditionType = "VirtualRouterActive"
	RoutesActive                        VirtualServiceConditionType = "RoutesActive"
	VirtualServiceMeshMarkedForDeletion VirtualServiceConditionType = "MeshMarkedForDeletion"
)

type VirtualServiceCondition struct {
	// Type of mesh service condition.
	Type VirtualServiceConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status api.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason *string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message *string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualServiceList is a list of VirtualService resources
type VirtualServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VirtualService `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualNode is a specification for a VirtualNode resource
type VirtualNode struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec VirtualNodeSpec `json:"spec,omitempty"`
	// +optional
	Status VirtualNodeStatus `json:"status,omitempty"`
}

// VirtualNodeSpec is the spec for a VirtualNode resource
type VirtualNodeSpec struct {
	MeshName string `json:"meshName"`
	// +optional
	Listeners []Listener `json:"listeners,omitempty"`
	// +optional
	ServiceDiscovery *ServiceDiscovery `json:"serviceDiscovery,omitempty"`
	// +optional
	Backends []Backend `json:"backends,omitempty"`
	// +optional
	Logging *Logging `json:"logging,omitempty"`
}

type Listener struct {
	PortMapping PortMapping `json:"portMapping"`
}

type PortMapping struct {
	Port     int64  `json:"port"`
	Protocol string `json:"protocol"`
}

type ServiceDiscovery struct {
	// +optional
	CloudMap *CloudMapServiceDiscovery `json:"cloudMap,omitempty"`
	// +optional
	Dns *DnsServiceDiscovery `json:"dns,omitempty"`
}

type CloudMapServiceDiscovery struct {
	CloudMapServiceName string `json:"cloudMapServiceName"`
}

type DnsServiceDiscovery struct {
	HostName string `json:"hostName"`
}

type Backend struct {
	VirtualService VirtualServiceBackend `json:"virtualService"`
}

type VirtualServiceBackend struct {
	VirtualServiceName string `json:"virtualServiceName"`
}

// Logging refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_Logging.html
type Logging struct {
	AccessLog *AccessLog `json:"accessLog"`
}

// AccessLog refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_AccessLog.html
type AccessLog struct {
	File *FileAccessLog `json:"file"`
}

// FileAccessLog refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_FileAccessLog.html
type FileAccessLog struct {
	Path string `json:"path"`
}

// VirtualNodeStatus is the status for a VirtualNode resource
type VirtualNodeStatus struct {
	MeshArn *string `json:"meshArn,omitempty"`
	// VirtualNodeArn is the AppMesh VirtualNode object's Amazon Resource Name
	// +optional
	VirtualNodeArn *string `json:"virtualNodeArn,omitempty"`
	// CloudMapServiceArn is a CloudMap Service object's Amazon Resource Name
	// +optional
	CloudMapServiceArn *string `json:"cloudMapServiceArn,omitempty"`
	// +optional
	QueryParameters map[string]string      `json:"queryParameters,omitempty"`
	Conditions      []VirtualNodeCondition `json:"conditions"`
}

type VirtualNodeConditionType string

const (
	// VirtualNodeActive is Active when the Appmesh Node has been created or found via the API
	VirtualNodeActive                VirtualNodeConditionType = "VirtualNodeActive"
	VirtualNodeMeshMarkedForDeletion VirtualNodeConditionType = "MeshMarkedForDeletion"
)

type VirtualNodeCondition struct {
	// Type of mesh node condition.
	Type VirtualNodeConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status api.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason *string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message *string `json:"reason,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualNodeList is a list of VirtualNode resources
type VirtualNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VirtualNode `json:"items"`
}
