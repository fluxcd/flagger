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

type VirtualGatewayConditionType string

const (
	// VirtualGatewayActive is True when the AppMesh VirtualGateway has been created or found via the API
	VirtualGatewayActive VirtualGatewayConditionType = "VirtualGatewayActive"
)

// +kubebuilder:validation:Enum=grpc;http;http2
type VirtualGatewayPortProtocol string

const (
	VirtualGatewayPortProtocolGRPC  VirtualGatewayPortProtocol = "grpc"
	VirtualGatewayPortProtocolHTTP  VirtualGatewayPortProtocol = "http"
	VirtualGatewayPortProtocolHTTP2 VirtualGatewayPortProtocol = "http2"
	VirtualGatewayPortProtocolTCP   VirtualGatewayPortProtocol = "tcp"
)

// VirtualGatewayPortMapping refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayPortMapping struct {
	// The port used for the port mapping.
	Port PortNumber `json:"port"`
	// The protocol used for the port mapping.
	Protocol VirtualGatewayPortProtocol `json:"protocol"`
}

type VirtualGatewayCondition struct {
	// Type of VirtualGateway condition.
	Type VirtualGatewayConditionType `json:"type"`
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

// VirtualGatewayHealthCheckPolicy refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayHealthCheckPolicy struct {
	// The number of consecutive successful health checks that must occur before declaring listener healthy.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	HealthyThreshold int64 `json:"healthyThreshold"`
	// The time period in milliseconds between each health check execution.
	// +kubebuilder:validation:Minimum=5000
	// +kubebuilder:validation:Maximum=300000
	IntervalMillis int64 `json:"intervalMillis"`
	// The destination path for the health check request.
	// This value is only used if the specified protocol is http or http2. For any other protocol, this value is ignored.
	// +optional
	Path *string `json:"path,omitempty"`
	// The destination port for the health check request.
	// +optional
	Port *PortNumber `json:"port,omitempty"`
	// The protocol for the health check request
	Protocol VirtualGatewayPortProtocol `json:"protocol"`
	// The amount of time to wait when receiving a response from the health check, in milliseconds.
	// +kubebuilder:validation:Minimum=2000
	// +kubebuilder:validation:Maximum=60000
	TimeoutMillis int64 `json:"timeoutMillis"`
	// The number of consecutive failed health checks that must occur before declaring a virtual Gateway unhealthy.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	UnhealthyThreshold int64 `json:"unhealthyThreshold"`
}

// VirtualGatewayConnectionPool refers to the connection pools settings for Virtual Gateway.
// Connection pool limits the number of connections that an Envoy can concurrently establish with
// all the hosts in the upstream cluster. Currently connection pool is supported only at the listener
// level and it is intended protect your local application from being overwhelmed with connections.
type VirtualGatewayConnectionPool struct {
	// Specifies http connection pool settings for the virtual gateway listener
	// +optional
	HTTP *HTTPConnectionPool `json:"http,omitempty"`
	// Specifies http2 connection pool settings for the virtual gateway listener
	// +optional
	HTTP2 *HTTP2ConnectionPool `json:"http2,omitempty"`
	// Specifies grpc connection pool settings for the virtual gateway listener
	// +optional
	GRPC *GRPCConnectionPool `json:"grpc,omitempty"`
}

// VirtualGatewayListenerTLSACMCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayListenerTLSACMCertificate struct {
	// The Amazon Resource Name (ARN) for the certificate.
	CertificateARN string `json:"certificateARN"`
}

// VirtualGatewayListenerTLSFileCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayListenerTLSFileCertificate struct {
	// The certificate chain for the certificate.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	CertificateChain string `json:"certificateChain"`
	// The private key for a certificate stored on the file system of the virtual Gateway.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	PrivateKey string `json:"privateKey"`
}

type VirtualGatewayListenerTLSSDSCertificate struct {
	// The certificate trust chain for a certificate issued via SDS cluster
	SecretName *string `json:"secretName"`
}

type VirtualGatewayListenerTLSValidationContextTrust struct {
	// A reference to an object that represents a TLS validation context trust for an AWS Certicate Manager (ACM) certificate.
	// +optional
	ACM *VirtualGatewayTLSValidationContextACMTrust `json:"acm,omitempty"`
	// An object that represents a TLS validation context trust for a local file.
	// +optional
	File *VirtualGatewayTLSValidationContextFileTrust `json:"file,omitempty"`
	// An object that represents a TLS validation context trust for an SDS system
	// +optional
	SDS *VirtualGatewayTLSValidationContextSDSTrust `json:"sds,omitempty"`
}

// VirtualGatewayListenerTLSCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayListenerTLSCertificate struct {
	// A reference to an object that represents an AWS Certificate Manager (ACM) certificate.
	// +optional
	ACM *VirtualGatewayListenerTLSACMCertificate `json:"acm,omitempty"`
	// A reference to an object that represents a local file certificate.
	// +optional
	File *VirtualGatewayListenerTLSFileCertificate `json:"file,omitempty"`
	// A reference to an object that represents an SDS issued certificate
	// +optional
	SDS *VirtualGatewayListenerTLSSDSCertificate `json:"sds,omitempty"`
}

// VirtualGatewayListenerTLSCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayListenerTLSValidationContext struct {
	Trust VirtualGatewayListenerTLSValidationContextTrust `json:"trust"`
	// Possible alternate names to consider
	// +optional
	SubjectAlternativeNames *SubjectAlternativeNames `json:"subjectAlternativeNames,omitempty"`
}

const (
	VirtualGatewayListenerTLSModeDisabled   VirtualGatewayListenerTLSMode = "DISABLED"
	VirtualGatewayListenerTLSModePermissive VirtualGatewayListenerTLSMode = "PERMISSIVE"
	VirtualGatewayListenerTLSModeStrict     VirtualGatewayListenerTLSMode = "STRICT"
)

// +kubebuilder:validation:Enum=DISABLED;PERMISSIVE;STRICT
type VirtualGatewayListenerTLSMode string

// VirtualGatewayListenerTLS refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayListenerTLS struct {
	// A reference to an object that represents a listener's TLS certificate.
	Certificate VirtualGatewayListenerTLSCertificate `json:"certificate"`
	// A reference to an object that represents Validation context
	// +optional
	Validation *VirtualGatewayListenerTLSValidationContext `json:"validation,omitempty"`
	// ListenerTLS mode
	Mode VirtualGatewayListenerTLSMode `json:"mode"`
}

// VirtualGatewayFileAccessLog refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayFileAccessLog struct {
	// The file path to write access logs to.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Path string `json:"path"`
}

// VirtualGatewayAccessLog refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayAccessLog struct {
	// The file object to send virtual gateway access logs to.
	// +optional
	File *VirtualGatewayFileAccessLog `json:"file,omitempty"`
}

// VirtualGatewayLogging refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayLogging struct {
	// The access log configuration for a virtual Gateway.
	// +optional
	AccessLog *VirtualGatewayAccessLog `json:"accessLog,omitempty"`
}

// VirtualGatewayListener refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayListener struct {
	// The port mapping information for the listener.
	PortMapping VirtualGatewayPortMapping `json:"portMapping"`
	// The health check information for the listener.
	// +optional
	HealthCheck *VirtualGatewayHealthCheckPolicy `json:"healthCheck,omitempty"`
	// The connection pool settings for the listener
	// +optional
	ConnectionPool *VirtualGatewayConnectionPool `json:"connectionPool,omitempty"`
	// A reference to an object that represents the Transport Layer Security (TLS) properties for a listener.
	// +optional
	TLS *VirtualGatewayListenerTLS `json:"tls,omitempty"`
}

// VirtualGatewayTLSValidationContextACMTrust refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayTLSValidationContextACMTrust struct {
	// One or more ACM Amazon Resource Name (ARN)s.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	CertificateAuthorityARNs []string `json:"certificateAuthorityARNs"`
}

// VirtualGatewayTLSValidationContextFileTrust refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayTLSValidationContextFileTrust struct {
	// The certificate trust chain for a certificate stored on the file system of the virtual Gateway.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	CertificateChain string `json:"certificateChain"`
}

// VirtualGatewayTLSValidationContextSDSTrust refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayTLSValidationContextSDSTrust struct {
	// The certificate trust chain for a certificate issued via SDS.
	SecretName *string `json:"secretName"`
}

// VirtualGatewayTLSValidationContextTrust refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayTLSValidationContextTrust struct {
	// A reference to an object that represents a TLS validation context trust for an AWS Certicate Manager (ACM) certificate.
	// +optional
	ACM *VirtualGatewayTLSValidationContextACMTrust `json:"acm,omitempty"`
	// An object that represents a TLS validation context trust for a local file.
	// +optional
	File *VirtualGatewayTLSValidationContextFileTrust `json:"file,omitempty"`
	// An object that represents a TLS validation context trust for a SDS certificate
	// +optional
	SDS *VirtualGatewayTLSValidationContextSDSTrust `json:"sds,omitempty"`
}

// VirtualGatewayTLSValidationContext refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayTLSValidationContext struct {
	// A reference to an object that represents a TLS validation context trust
	Trust VirtualGatewayTLSValidationContextTrust `json:"trust"`
	// Possible alternative names to consider
	// +optional
	SubjectAlternativeNames *SubjectAlternativeNames `json:"subjectAlternativeNames,omitempty"`
}

// VirtualGatewayTLSValidationContext refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayClientTLSCertificate struct {
	// An object that represents a TLS cert via a local file
	// +optional
	File *VirtualGatewayListenerTLSFileCertificate `json:"file,omitempty"`
	// An object that represents a TLS cert via SDS entry
	// +optional
	SDS *VirtualGatewayListenerTLSSDSCertificate `json:"sds,omitempty"`
}

// VirtualGatewayClientPolicyTLS refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayClientPolicyTLS struct {
	// Whether the policy is enforced.
	// If unspecified, default settings from AWS API will be applied. Refer to AWS Docs for default settings.
	// +optional
	Enforce *bool `json:"enforce,omitempty"`
	// The range of ports that the policy is enforced for.
	// +optional
	Ports []PortNumber `json:"ports,omitempty"`
	// A reference to an object that represents TLS certificate.
	//+optional
	Certificate *VirtualGatewayClientTLSCertificate `json:"certificate,omitempty"`
	// A reference to an object that represents a TLS validation context.
	Validation VirtualGatewayTLSValidationContext `json:"validation"`
}

// VirtualGatewayClientPolicy refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayClientPolicy struct {
	// A reference to an object that represents a Transport Layer Security (TLS) client policy.
	// +optional
	TLS *VirtualGatewayClientPolicyTLS `json:"tls,omitempty"`
}

// VirtualGatewayBackendDefaults refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewayBackendDefaults struct {
	// A reference to an object that represents a client policy.
	// +optional
	ClientPolicy *VirtualGatewayClientPolicy `json:"clientPolicy,omitempty"`
}

// VirtualGatewaySpec defines the desired state of VirtualGateway
// refers to https://docs.aws.amazon.com/app-mesh/latest/userguide/virtual_gateways.html
type VirtualGatewaySpec struct {
	// AWSName is the AppMesh VirtualGateway object's name.
	// If unspecified or empty, it defaults to be "${name}_${namespace}" of k8s VirtualGateway
	// +optional
	AWSName *string `json:"awsName,omitempty"`
	// NamespaceSelector selects Namespaces using labels to designate GatewayRoute membership.
	// This field follows standard label selector semantics; if present but empty, it selects all namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	// PodSelector selects Pods using labels to designate VirtualGateway membership.
	// This field follows standard label selector semantics:
	//	if present but empty, it selects all pods within namespace.
	// 	if absent, it selects no pod.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// GatewayRouteSelector selects GatewayRoutes using labels to designate GatewayRoute membership.
	// If not specified it selects all GatewayRoutes in that namespace.
	// +optional
	GatewayRouteSelector *metav1.LabelSelector `json:"gatewayRouteSelector,omitempty"`
	// The listener that the virtual gateway is expected to receive inbound traffic from
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=1
	Listeners []VirtualGatewayListener `json:"listeners,omitempty"`
	// The inbound and outbound access logging information for the virtual gateway.
	// +optional
	Logging *VirtualGatewayLogging `json:"logging,omitempty"`
	// A reference to an object that represents the defaults for backend GatewayRoutes.
	// +optional
	BackendDefaults *VirtualGatewayBackendDefaults `json:"backendDefaults,omitempty"`

	// A reference to k8s Mesh CR that this VirtualGateway belongs to.
	// The admission controller populates it using Meshes's selector, and prevents users from setting this field.
	//
	// Populated by the system.
	// Read-only.
	// +optional
	MeshRef *MeshReference `json:"meshRef,omitempty"`
}

// VirtualGatewayStatus defines the observed state of VirtualGateway
type VirtualGatewayStatus struct {
	// VirtualGatewayARN is the AppMesh VirtualGateway object's Amazon Resource Name
	// +optional
	VirtualGatewayARN *string `json:"virtualGatewayARN,omitempty"`
	// The current VirtualGateway status.
	// +optional
	Conditions []VirtualGatewayCondition `json:"conditions,omitempty"`

	// The generation observed by the VirtualGateway controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.virtualGatewayARN",description="The AppMesh VirtualGateway object's Amazon Resource Name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// VirtualGateway is the Schema for the virtualgateways API
type VirtualGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualGatewaySpec   `json:"spec,omitempty"`
	Status VirtualGatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualGatewayList contains a list of VirtualGateway
type VirtualGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualGateway `json:"items"`
}

/**
func init() {
	SchemeBuilder.Register(&VirtualGateway{}, &VirtualGatewayList{})
}
**/
