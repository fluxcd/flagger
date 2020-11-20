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

// VirtualRouterListener refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualRouterListener.html
type VirtualRouterListener struct {
	// The port mapping information for the listener.
	PortMapping PortMapping `json:"portMapping"`
}

// WeightedTarget refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_WeightedTarget.html
type WeightedTarget struct {
	// Reference to Kubernetes VirtualNode CR in cluster to associate with the weighted target. Exactly one of 'virtualNodeRef' or 'virtualNodeARN' must be specified.
	// +optional
	VirtualNodeRef *VirtualNodeReference `json:"virtualNodeRef,omitempty"`
	// Amazon Resource Name to AppMesh VirtualNode object to associate with the weighted target. Exactly one of 'virtualNodeRef' or 'virtualNodeARN' must be specified.
	// +optional
	VirtualNodeARN *string `json:"virtualNodeARN,omitempty"`
	// The relative weight of the weighted target.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight int64 `json:"weight"`
}

type MatchRange struct {
	// The start of the range.
	// +optional
	Start int64 `json:"start"`
	// The end of the range.
	// +optional
	End int64 `json:"end"`
}

// HeaderMatchMethod refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HeaderMatchMethod.html
type HeaderMatchMethod struct {
	// The value sent by the client must match the specified value exactly.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Exact *string `json:"exact,omitempty"`
	// The value sent by the client must begin with the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Prefix *string `json:"prefix,omitempty"`
	// An object that represents the range of values to match on.
	// +optional
	Range *MatchRange `json:"range,omitempty"`
	// The value sent by the client must include the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Regex *string `json:"regex,omitempty"`
	// The value sent by the client must end with the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Suffix *string `json:"suffix,omitempty"`
}

// HTTPRouteHeader refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HttpRouteHeader.html
type HTTPRouteHeader struct {
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

// HTTPRouteMatch refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HttpRouteMatch.html
type HTTPRouteMatch struct {
	// An object that represents the client request headers to match on.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Headers []HTTPRouteHeader `json:"headers,omitempty"`
	// The client request method to match on.
	// +kubebuilder:validation:Enum=CONNECT;DELETE;GET;HEAD;OPTIONS;PATCH;POST;PUT;TRACE
	// +optional
	Method *string `json:"method,omitempty"`
	// Specifies the path to match requests with
	Prefix string `json:"prefix"`
	// The client request scheme to match on
	// +kubebuilder:validation:Enum=http;https
	// +optional
	Scheme *string `json:"scheme,omitempty"`
}

// HTTPRouteAction refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HttpRouteAction.html
type HTTPRouteAction struct {
	// An object that represents the targets that traffic is routed to when a request matches the route.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	WeightedTargets []WeightedTarget `json:"weightedTargets"`
}

// +kubebuilder:validation:Enum=server-error;gateway-error;client-error;stream-error
type HTTPRetryPolicyEvent string

// +kubebuilder:validation:Enum=connection-error
type TCPRetryPolicyEvent string

// +kubebuilder:validation:Enum=cancelled;deadline-exceeded;internal;resource-exhausted;unavailable
type GRPCRetryPolicyEvent string

// HTTPRetryPolicy refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HttpRetryPolicy.html
type HTTPRetryPolicy struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=25
	// +optional
	HTTPRetryEvents []HTTPRetryPolicyEvent `json:"httpRetryEvents,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	// +optional
	TCPRetryEvents []TCPRetryPolicyEvent `json:"tcpRetryEvents,omitempty"`
	// The maximum number of retry attempts.
	// +kubebuilder:validation:Minimum=0
	MaxRetries int64 `json:"maxRetries"`
	// An object that represents a duration of time
	PerRetryTimeout Duration `json:"perRetryTimeout"`
}

// HTTPRoute refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HttpRoute.html
type HTTPRoute struct {
	// An object that represents the criteria for determining a request match.
	Match HTTPRouteMatch `json:"match"`
	// An object that represents the action to take if a match is determined.
	Action HTTPRouteAction `json:"action"`
	// An object that represents a retry policy.
	// +optional
	RetryPolicy *HTTPRetryPolicy `json:"retryPolicy,omitempty"`
	// An object that represents a http timeout.
	// +optional
	Timeout *HTTPTimeout `json:"timeout,omitempty"`
}

// TCPRouteAction refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_TcpRouteAction.html
type TCPRouteAction struct {
	// An object that represents the targets that traffic is routed to when a request matches the route.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	WeightedTargets []WeightedTarget `json:"weightedTargets"`
}

// TCPRoute refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_TcpRoute.html
type TCPRoute struct {
	// The action to take if a match is determined.
	Action TCPRouteAction `json:"action"`
	// An object that represents a tcp timeout.
	// +optional
	Timeout *TCPTimeout `json:"timeout,omitempty"`
}

// GRPCRouteMetadataMatchMethod refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRouteMetadataMatchMethod.html
type GRPCRouteMetadataMatchMethod struct {
	// The value sent by the client must match the specified value exactly.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Exact *string `json:"exact,omitempty"`
	// The value sent by the client must begin with the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Prefix *string `json:"prefix,omitempty"`
	// An object that represents the range of values to match on
	// +optional
	Range *MatchRange `json:"range,omitempty"`
	// The value sent by the client must include the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Regex *string `json:"regex,omitempty"`
	// The value sent by the client must end with the specified characters.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Suffix *string `json:"suffix,omitempty"`
}

// GRPCRouteMetadata refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRouteMetadata.html
type GRPCRouteMetadata struct {
	// The name of the route.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	Name string `json:"name"`
	// An object that represents the data to match from the request.
	// +optional
	Match *GRPCRouteMetadataMatchMethod `json:"match,omitempty"`
	// Specify True to match anything except the match criteria. The default value is False.
	// +optional
	Invert *bool `json:"invert,omitempty"`
}

// GRPCRouteMatch refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRouteMatch.html
type GRPCRouteMatch struct {
	// The method name to match from the request. If you specify a name, you must also specify a serviceName.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	// +optional
	MethodName *string `json:"methodName,omitempty"`
	// The fully qualified domain name for the service to match from the request.
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`
	// An object that represents the data to match from the request.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Metadata []GRPCRouteMetadata `json:"metadata,omitempty"`
}

// GRPCRouteAction refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRouteAction.html
type GRPCRouteAction struct {
	// An object that represents the targets that traffic is routed to when a request matches the route.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	WeightedTargets []WeightedTarget `json:"weightedTargets"`
}

// GRPCRetryPolicy refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRetryPolicy.html
type GRPCRetryPolicy struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	// +optional
	GRPCRetryEvents []GRPCRetryPolicyEvent `json:"grpcRetryEvents,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=25
	// +optional
	HTTPRetryEvents []HTTPRetryPolicyEvent `json:"httpRetryEvents,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	// +optional
	TCPRetryEvents []TCPRetryPolicyEvent `json:"tcpRetryEvents,omitempty"`
	// The maximum number of retry attempts.
	// +kubebuilder:validation:Minimum=0
	MaxRetries int64 `json:"maxRetries"`
	// An object that represents a duration of time.
	PerRetryTimeout Duration `json:"perRetryTimeout"`
}

// GRPCRoute refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_GrpcRoute.html
type GRPCRoute struct {
	// An object that represents the criteria for determining a request match.
	Match GRPCRouteMatch `json:"match"`
	// An object that represents the action to take if a match is determined.
	Action GRPCRouteAction `json:"action"`
	// An object that represents a retry policy.
	// +optional
	RetryPolicy *GRPCRetryPolicy `json:"retryPolicy,omitempty"`
	// An object that represents a grpc timeout.
	// +optional
	Timeout *GRPCTimeout `json:"timeout,omitempty"`
}

// Route refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_RouteSpec.html
type Route struct {
	// Route's name
	Name string `json:"name"`
	// An object that represents the specification of a gRPC route.
	// +optional
	GRPCRoute *GRPCRoute `json:"grpcRoute,omitempty"`
	// An object that represents the specification of an HTTP route.
	// +optional
	HTTPRoute *HTTPRoute `json:"httpRoute,omitempty"`
	// An object that represents the specification of an HTTP/2 route.
	// +optional
	HTTP2Route *HTTPRoute `json:"http2Route,omitempty"`
	// An object that represents the specification of a TCP route.
	// +optional
	TCPRoute *TCPRoute `json:"tcpRoute,omitempty"`
	// The priority for the route.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority *int64 `json:"priority,omitempty"`
}

type VirtualRouterConditionType string

const (
	// VirtualRouterActive is True when the AppMesh VirtualRouter has been created or found via the API
	VirtualRouterActive VirtualRouterConditionType = "VirtualRouterActive"
)

type VirtualRouterCondition struct {
	// Type of VirtualRouter condition.
	Type VirtualRouterConditionType `json:"type"`
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

// VirtualRouterSpec defines the desired state of VirtualRouter
// refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualRouterSpec.html
type VirtualRouterSpec struct {
	// AWSName is the AppMesh VirtualRouter object's name.
	// If unspecified or empty, it defaults to be "${name}_${namespace}" of k8s VirtualRouter
	// +optional
	AWSName *string `json:"awsName,omitempty"`
	// The listeners that the virtual router is expected to receive inbound traffic from
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	Listeners []VirtualRouterListener `json:"listeners,omitempty"`

	// The routes associated with VirtualRouter
	// +optional
	Routes []Route `json:"routes,omitempty"`

	// A reference to k8s Mesh CR that this VirtualRouter belongs to.
	// The admission controller populates it using Meshes's selector, and prevents users from setting this field.
	//
	// Populated by the system.
	// Read-only.
	// +optional
	MeshRef *MeshReference `json:"meshRef,omitempty"`
}

// VirtualRouterStatus defines the observed state of VirtualRouter
type VirtualRouterStatus struct {
	// VirtualRouterARN is the AppMesh VirtualRouter object's Amazon Resource Name.
	// +optional
	VirtualRouterARN *string `json:"virtualRouterARN,omitempty"`
	// RouteARNs is a map of AppMesh Route objects' Amazon Resource Names, indexed by route name.
	// +optional
	RouteARNs map[string]string `json:"routeARNs,omitempty"`
	// The current VirtualRouter status.
	// +optional
	Conditions []VirtualRouterCondition `json:"conditions,omitempty"`

	// The generation observed by the VirtualRouter controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualRouter is the Schema for the virtualrouters API
type VirtualRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualRouterSpec   `json:"spec,omitempty"`
	Status VirtualRouterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualRouterList contains a list of VirtualRouter
type VirtualRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualRouter `json:"items"`
}
