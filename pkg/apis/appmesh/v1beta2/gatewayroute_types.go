/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayRouteVirtualService refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type GatewayRouteVirtualService struct {
	// Reference to Kubernetes VirtualService CR in cluster to associate with the gateway route virtual service target. Exactly one of 'virtualServiceRef' or 'virtualServiceARN' must be specified.
	// +optional
	VirtualServiceRef *VirtualServiceReference `json:"virtualServiceRef,omitempty"`
	// Amazon Resource Name to AppMesh VirtualService object to associate with the gateway route virtual service target. Exactly one of 'virtualServiceRef' or 'virtualServiceARN' must be specified.
	// +optional
	VirtualServiceARN *string `json:"virtualServiceARN,omitempty"`
}

// GatewayRouteTarget refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type GatewayRouteTarget struct {
	// The virtual service to associate with the gateway route target.
	VirtualService GatewayRouteVirtualService `json:"virtualService"`
}

// GRPCGatewayRouteMatch refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type GRPCGatewayRouteMatch struct {
	// Either ServiceName or Hostname must be specified. Both are allowed as well
	// The fully qualified domain name for the service to match from the request.
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`
	// The client specified Hostname to match on.
	// +optional
	Hostname *GatewayRouteHostnameMatch `json:"hostname,omitempty"`
	// An object that represents the data to match from the request.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Metadata []GRPCGatewayRouteMetadata `json:"metadata,omitempty"`
}

// GRPCGatewayRouteMetadata refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRouteMetadata.html
type GRPCGatewayRouteMetadata struct {
	// The name of the route.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	Name *string `json:"name"`
	// An object that represents the data to match from the request.
	// +optional
	Match *GRPCRouteMetadataMatchMethod `json:"match,omitempty"`
	// Specify True to match anything except the match criteria. The default value is False.
	// +optional
	Invert *bool `json:"invert,omitempty"`
}

// GRPCGatewayRouteAction refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type GRPCGatewayRouteAction struct {
	// An object that represents the target that traffic is routed to when a request matches the route.
	Target GatewayRouteTarget `json:"target"`
	// +optional
	Rewrite *GrpcGatewayRouteRewrite `json:"rewrite,omitempty"`
}

type GrpcGatewayRouteRewrite struct {
	Hostname *GatewayRouteHostnameRewrite `json:"hostname,omitempty"`
}

// GRPCGatewayRoute refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type GRPCGatewayRoute struct {
	// An object that represents the criteria for determining a request match.
	Match GRPCGatewayRouteMatch `json:"match"`
	// An object that represents the action to take if a match is determined.
	Action GRPCGatewayRouteAction `json:"action"`
}

// HTTPGatewayRouteMatch refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type HTTPGatewayRouteMatch struct {
	// Either Prefix or Hostname must be specified. Both are allowed as well.
	// Specifies the path to match requests with
	// +optional
	Prefix *string `json:"prefix,omitempty"`
	// +optional
	Path *HTTPPathMatch `json:"path,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	QueryParameters []HTTPQueryParameters `json:"queryParameters,omitempty"`
	// The client request method to match on.
	// +kubebuilder:validation:Enum=CONNECT;DELETE;GET;HEAD;OPTIONS;PATCH;POST;PUT;TRACE
	// +optional
	Method *string `json:"method,omitempty"`
	// The client specified Hostname to match on.
	// +optional
	Hostname *GatewayRouteHostnameMatch `json:"hostname,omitempty"`
	// An object that represents the client request headers to match on.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Headers []HTTPGatewayRouteHeader `json:"headers,omitempty"`
}

// HTTPGatewayRouteHeader refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HttpRouteHeader.html
type HTTPGatewayRouteHeader struct {
	// A name for the HTTP header in the client request that will be matched on.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	Name string `json:"name"`
	// The HeaderMatchMethod object.
	// +optional
	Match *HeaderMatchMethod `json:"match,omitempty"`
	// Specify True to match anything except the match criteria. The default value is False.
	// +optional
	Invert *bool `json:"invert,omitempty"`
}

// Hostname based match, either Exact or Suffix must be specified. Both are not allowed
type GatewayRouteHostnameMatch struct {
	// The value sent by the client must match the specified value exactly.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Exact *string `json:"exact,omitempty"`
	// The value sent by the client must end with the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Suffix *string `json:"suffix,omitempty"`
}

// HTTPGatewayRouteAction refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type HTTPGatewayRouteAction struct {
	// An object that represents the target that traffic is routed to when a request matches the route.
	Target GatewayRouteTarget `json:"target"`
	// +optional
	Rewrite *HTTPGatewayRouteRewrite `json:"rewrite,omitempty"`
}

type HTTPGatewayRouteRewrite struct {
	// +optional
	Prefix *GatewayRoutePrefixRewrite `json:"prefix,omitempty"`
	// +optional
	Path *GatewayRoutePathRewrite `json:"path,omitempty"`
	//+optional
	Hostname *GatewayRouteHostnameRewrite `json:"hostname,omitempty"`
}

type GatewayRoutePrefixRewrite struct {
	// +optional
	// +kubebuilder:validation:Enum=ENABLED;DISABLED;
	DefaultPrefix *string `json:"defaultPrefix,omitempty"`
	// When DefaultPrefix is specified, Value cannot be set
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Value *string `json:"value,omitempty"`
}

// ENABLE or DISABLE default behavior for Hostname rewrite
type GatewayRouteHostnameRewrite struct {
	// +optional
	// +kubebuilder:validation:Enum=ENABLED;DISABLED;
	DefaultTargetHostname *string `json:"defaultTargetHostname,omitempty"`
}

type GatewayRoutePathRewrite struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Exact *string `json:"exact,omitempty"`
}

// HTTPGatewayRoute refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type HTTPGatewayRoute struct {
	// An object that represents the criteria for determining a request match.
	Match HTTPGatewayRouteMatch `json:"match"`
	// An object that represents the action to take if a match is determined.
	Action HTTPGatewayRouteAction `json:"action"`
}

// GatewayRouteSpec defines the desired state of GatewayRoute
// refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type GatewayRouteSpec struct {
	// AWSName is the AppMesh GatewayRoute object's name.
	// If unspecified or empty, it defaults to be "${name}_${namespace}" of k8s GatewayRoute
	// +optional
	AWSName *string `json:"awsName,omitempty"`
	// Priority for the gatewayroute.
	// Default Priority is 1000 which is lowest priority
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority *int64 `json:"priority,omitempty"`
	// An object that represents the specification of a gRPC gatewayRoute.
	// +optional
	GRPCRoute *GRPCGatewayRoute `json:"grpcRoute,omitempty"`
	// An object that represents the specification of an HTTP gatewayRoute.
	// +optional
	HTTPRoute *HTTPGatewayRoute `json:"httpRoute,omitempty"`
	// An object that represents the specification of an HTTP/2 gatewayRoute.
	// +optional
	HTTP2Route *HTTPGatewayRoute `json:"http2Route,omitempty"`
	// A reference to k8s VirtualGateway CR that this GatewayRoute belongs to.
	// The admission controller populates it using VirtualGateway's selector, and prevents users from setting this field.
	//
	// Populated by the system.
	// Read-only.
	// +optional
	VirtualGatewayRef *VirtualGatewayReference `json:"virtualGatewayRef,omitempty"`
	// A reference to k8s Mesh CR that this GatewayRoute belongs to.
	// The admission controller populates it using Meshes's selector, and prevents users from setting this field.
	//
	// Populated by the system.
	// Read-only.
	// +optional
	MeshRef *MeshReference `json:"meshRef,omitempty"`
}

type GatewayRouteConditionType string

const (
	// GatewayRouteActive is True when the AppMesh GatewayRoute has been created or found via the API
	GatewayRouteActive GatewayRouteConditionType = "GatewayRouteActive"
)

type GatewayRouteCondition struct {
	// Type of GatewayRoute condition.
	Type GatewayRouteConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
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

// GatewayRouteStatus defines the observed state of GatewayRoute
type GatewayRouteStatus struct {
	// GatewayRouteARN is the AppMesh GatewayRoute object's Amazon Resource Name
	// +optional
	GatewayRouteARN *string `json:"gatewayRouteARN,omitempty"`
	// The current GatewayRoute status.
	// +optional
	Conditions []GatewayRouteCondition `json:"conditions,omitempty"`
	// The generation observed by the GatewayRoute controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.gatewayRouteARN",description="The AppMesh GatewayRoute object's Amazon Resource Name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// GatewayRoute is the Schema for the gatewayroutes API
type GatewayRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayRouteSpec   `json:"spec,omitempty"`
	Status GatewayRouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayRouteList contains a list of GatewayRoute
type GatewayRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayRoute `json:"items"`
}

/**
func init() {
	SchemeBuilder.Register(&GatewayRoute{}, &GatewayRouteList{})
}
**/
