package v1beta2

import "k8s.io/apimachinery/pkg/types"

// +kubebuilder:validation:Enum=s;ms
type DurationUnit string

const (
	DurationUnitS  DurationUnit = "s"
	DurationUnitMS DurationUnit = "ms"
)

type Duration struct {
	// A unit of time.
	Unit DurationUnit `json:"unit"`
	// A number of time units.
	// +kubebuilder:validation:Minimum=0
	Value int64 `json:"value"`
}

// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:Maximum=65535
type PortNumber int64

// +kubebuilder:validation:Enum=grpc;http;http2;tcp
type PortProtocol string

const (
	PortProtocolGRPC  PortProtocol = "grpc"
	PortProtocolHTTP  PortProtocol = "http"
	PortProtocolHTTP2 PortProtocol = "http2"
	PortProtocolTCP   PortProtocol = "tcp"
)

// PortMapping refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_PortMapping.html
type PortMapping struct {
	// The port used for the port mapping.
	Port PortNumber `json:"port"`
	// The protocol used for the port mapping.
	Protocol PortProtocol `json:"protocol"`
}

// VirtualNodeReference holds a reference to VirtualNode.appmesh.k8s.aws
type VirtualNodeReference struct {
	// Namespace is the namespace of VirtualNode CR.
	// If unspecified, defaults to the referencing object's namespace
	// +optional
	Namespace *string `json:"namespace,omitempty"`
	// Name is the name of VirtualNode CR
	Name string `json:"name"`
}

// VirtualServiceReference holds a reference to VirtualService.appmesh.k8s.aws
type VirtualServiceReference struct {
	// Namespace is the namespace of VirtualService CR.
	// If unspecified, defaults to the referencing object's namespace
	// +optional
	Namespace *string `json:"namespace,omitempty"`
	// Name is the name of VirtualService CR
	Name string `json:"name"`
}

// VirtualRouterReference holds a reference to VirtualRouter.appmesh.k8s.aws
type VirtualRouterReference struct {
	// Namespace is the namespace of VirtualRouter CR.
	// If unspecified, defaults to the referencing object's namespace
	// +optional
	Namespace *string `json:"namespace,omitempty"`
	// Name is the name of VirtualRouter CR
	Name string `json:"name"`
}

// MeshReference holds a reference to Mesh.appmesh.k8s.aws
type MeshReference struct {
	// Name is the name of Mesh CR
	Name string `json:"name"`
	// UID is the UID of Mesh CR
	UID types.UID `json:"uid"`
}

// VirtualGatewayReference holds a reference to VirtualGateway.appmesh.k8s.aws
type VirtualGatewayReference struct {
	// Namespace is the namespace of VirtualGateway CR.
	// If unspecified, defaults to the referencing object's namespace
	// +optional
	Namespace *string `json:"namespace,omitempty"`
	// Name is the name of VirtualGateway CR
	Name string `json:"name"`
	// UID is the UID of VirtualGateway CR
	UID types.UID `json:"uid"`
}

type HTTPTimeout struct {
	// An object that represents per request timeout duration.
	// +optional
	PerRequest *Duration `json:"perRequest,omitempty"`
	// An object that represents idle timeout duration.
	// +optional
	Idle *Duration `json:"idle,omitempty"`
}

type GRPCTimeout struct {
	// An object that represents per request timeout duration.
	// +optional
	PerRequest *Duration `json:"perRequest,omitempty"`
	// An object that represents idle timeout duration.
	// +optional
	Idle *Duration `json:"idle,omitempty"`
}

type TCPTimeout struct {
	// An object that represents idle timeout duration.
	// +optional
	Idle *Duration `json:"idle,omitempty"`
}
