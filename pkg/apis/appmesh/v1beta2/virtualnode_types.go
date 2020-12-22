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

// TLSValidationContextACMTrust refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_TlsValidationContextAcmTrust.html
type TLSValidationContextACMTrust struct {
	// One or more ACM Amazon Resource Name (ARN)s.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	CertificateAuthorityARNs []string `json:"certificateAuthorityARNs"`
}

// TLSValidationContextFileTrust refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_TlsValidationContextFileTrust.html
type TLSValidationContextFileTrust struct {
	// The certificate trust chain for a certificate stored on the file system of the virtual node that the proxy is running on.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	CertificateChain string `json:"certificateChain"`
}

// TLSValidationContextTrust refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_TlsValidationContextTrust.html
type TLSValidationContextTrust struct {
	// A reference to an object that represents a TLS validation context trust for an AWS Certicate Manager (ACM) certificate.
	// +optional
	ACM *TLSValidationContextACMTrust `json:"acm,omitempty"`
	// An object that represents a TLS validation context trust for a local file.
	// +optional
	File *TLSValidationContextFileTrust `json:"file,omitempty"`
}

// TLSValidationContext refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_TlsValidationContext.html
type TLSValidationContext struct {
	// A reference to an object that represents a TLS validation context trust
	Trust TLSValidationContextTrust `json:"trust"`
}

// ClientPolicyTLS refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ClientPolicyTls.html
type ClientPolicyTLS struct {
	// Whether the policy is enforced.
	// If unspecified, default settings from AWS API will be applied. Refer to AWS Docs for default settings.
	// +optional
	Enforce *bool `json:"enforce,omitempty"`
	// The range of ports that the policy is enforced for.
	// +optional
	Ports []PortNumber `json:"ports,omitempty"`
	// A reference to an object that represents a TLS validation context.
	Validation TLSValidationContext `json:"validation"`
}

// ClientPolicy refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ClientPolicy.html
type ClientPolicy struct {
	// A reference to an object that represents a Transport Layer Security (TLS) client policy.
	// +optional
	TLS *ClientPolicyTLS `json:"tls,omitempty"`
}

// VirtualServiceBackend refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualServiceBackend.html
type VirtualServiceBackend struct {
	// Reference to Kubernetes VirtualService CR in cluster that is acting as a virtual node backend. Exactly one of 'virtualServiceRef' or 'virtualServiceARN' must be specified.
	// +optional
	VirtualServiceRef *VirtualServiceReference `json:"virtualServiceRef,omitempty"`
	// Amazon Resource Name to AppMesh VirtualService object that is acting as a virtual node backend. Exactly one of 'virtualServiceRef' or 'virtualServiceARN' must be specified.
	// +optional
	VirtualServiceARN *string `json:"virtualServiceARN,omitempty"`
	// A reference to an object that represents the client policy for a backend.
	// +optional
	ClientPolicy *ClientPolicy `json:"clientPolicy,omitempty"`
}

// Backend refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_Backend.html
type Backend struct {
	// Specifies a virtual service to use as a backend for a virtual node.
	VirtualService VirtualServiceBackend `json:"virtualService"`
}

// BackendDefaults refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_BackendDefaults.html
type BackendDefaults struct {
	// A reference to an object that represents a client policy.
	// +optional
	ClientPolicy *ClientPolicy `json:"clientPolicy,omitempty"`
}

// HealthCheckPolicy refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_HealthCheckPolicy.html
type HealthCheckPolicy struct {
	// The number of consecutive successful health checks that must occur before declaring listener healthy.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
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
	Protocol PortProtocol `json:"protocol"`
	// The amount of time to wait when receiving a response from the health check, in milliseconds.
	// +kubebuilder:validation:Minimum=2000
	// +kubebuilder:validation:Maximum=60000
	TimeoutMillis int64 `json:"timeoutMillis"`
	// The number of consecutive failed health checks that must occur before declaring a virtual node unhealthy.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	UnhealthyThreshold int64 `json:"unhealthyThreshold"`
}

// OutlierDetection defines the health check policy that temporarily ejects an endpoint/host of a VirtualNode
// from the load balancing set when it meets failure threshold
type OutlierDetection struct {
	// The threshold for the number of server errors returned by a given host during an outlier detection interval.
	// If the server error count meets/exceeds this threshold the host is ejected.
	// A server error is defined as any HTTP 5xx response (or the equivalent for gRPC and TCP connections)
	// +kubebuilder:validation:Minimum=1
	MaxServerErrors int64 `json:"maxServerErrors"`
	// The time interval between ejection analysis sweeps. This can result in both new ejections as well as hosts being returned to service
	Interval Duration `json:"interval"`
	// The base time that a host is ejected for. The real time is equal to the base time multiplied by the number of times the host has been ejected
	BaseEjectionDuration Duration `json:"baseEjectionDuration"`
	// The threshold for the max percentage of outlier hosts that can be ejected from the load balancing set.
	// maxEjectionPercent=100 means outlier detection can potentially eject all of the hosts from the upstream service if they are all considered outliers, leaving the load balancing set with zero hosts
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MaxEjectionPercent int64 `json:"maxEjectionPercent"`
}

// ListenerTLSACMCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ListenerTlsAcmCertificate.html
type ListenerTLSACMCertificate struct {
	// The Amazon Resource Name (ARN) for the certificate.
	CertificateARN string `json:"certificateARN"`
}

// ListenerTLSFileCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ListenerTlsFileCertificate.html
type ListenerTLSFileCertificate struct {
	// The certificate chain for the certificate.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	CertificateChain string `json:"certificateChain"`
	// The private key for a certificate stored on the file system of the virtual node that the proxy is running on.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	PrivateKey string `json:"privateKey"`
}

// ListenerTLSCertificate refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ListenerTlsCertificate.html
type ListenerTLSCertificate struct {
	// A reference to an object that represents an AWS Certificate Manager (ACM) certificate.
	// +optional
	ACM *ListenerTLSACMCertificate `json:"acm,omitempty"`
	// A reference to an object that represents a local file certificate.
	// +optional
	File *ListenerTLSFileCertificate `json:"file,omitempty"`
}

const (
	ListenerTLSModeDisabled   ListenerTLSMode = "DISABLED"
	ListenerTLSModePermissive ListenerTLSMode = "PERMISSIVE"
	ListenerTLSModeStrict     ListenerTLSMode = "STRICT"
)

// +kubebuilder:validation:Enum=DISABLED;PERMISSIVE;STRICT
type ListenerTLSMode string

// ListenerTLS refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ListenerTls.html
type ListenerTLS struct {
	// A reference to an object that represents a listener's TLS certificate.
	Certificate ListenerTLSCertificate `json:"certificate"`
	// ListenerTLS mode
	Mode ListenerTLSMode `json:"mode"`
}

// ListenerTimeout refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ListenerTimeout.html
type ListenerTimeout struct {
	// Specifies tcp timeout information for the virtual node.
	// +optional
	TCP *TCPTimeout `json:"tcp,omitempty"`
	// Specifies http timeout information for the virtual node.
	// +optional
	HTTP *HTTPTimeout `json:"http,omitempty"`
	// Specifies http2 information for the virtual node.
	// +optional
	HTTP2 *HTTPTimeout `json:"http2,omitempty"`
	// Specifies grpc timeout information for the virtual node.
	// +optional
	GRPC *GRPCTimeout `json:"grpc,omitempty"`
}

// VirtualNodeConnectionPool refers to the connection pools settings for Virtual Node.
// Connection pool limits the number of connections that an Envoy can concurrently establish with
// all the hosts in the upstream cluster. Currently connection pool is supported only at the listener
// level and it is intended protect your local application from being overwhelmed with connections.
type VirtualNodeConnectionPool struct {
	// Specifies tcp connection pool settings for the virtual node listener
	// +optional
	TCP *TCPConnectionPool `json:"tcp,omitempty"`
	// Specifies http connection pool settings for the virtual node listener
	// +optional
	HTTP *HTTPConnectionPool `json:"http,omitempty"`
	// Specifies http2 connection pool settings for the virtual node listener
	// +optional
	HTTP2 *HTTP2ConnectionPool `json:"http2,omitempty"`
	// Specifies grpc connection pool settings for the virtual node listener
	// +optional
	GRPC *GRPCConnectionPool `json:"grpc,omitempty"`
}

// Listener refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_Listener.html
type Listener struct {
	// The port mapping information for the listener.
	PortMapping PortMapping `json:"portMapping"`
	// The health check information for the listener.
	// +optional
	HealthCheck *HealthCheckPolicy `json:"healthCheck,omitempty"`
	// The outlier detection for the listener
	// +optional
	OutlierDetection *OutlierDetection `json:"outlierDetection,omitempty"`
	// The connection pool settings for the listener
	// +optional
	ConnectionPool *VirtualNodeConnectionPool `json:"connectionPool,omitempty"`
	// A reference to an object that represents the Transport Layer Security (TLS) properties for a listener.
	// +optional
	TLS *ListenerTLS `json:"tls,omitempty"`
	// A reference to an object that represents
	// +optional
	Timeout *ListenerTimeout `json:"timeout,omitempty"`
}

// AWSCloudMapInstanceAttribute refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_AwsCloudMapInstanceAttribute.html
type AWSCloudMapInstanceAttribute struct {
	// The name of an AWS Cloud Map service instance attribute key.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Key string `json:"key"`
	// The value of an AWS Cloud Map service instance attribute key.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Value string `json:"value"`
}

// AWSCloudMapServiceDiscovery refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_AwsCloudMapServiceDiscovery.html
type AWSCloudMapServiceDiscovery struct {
	// The name of the AWS Cloud Map namespace to use.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	NamespaceName string `json:"namespaceName"`
	// The name of the AWS Cloud Map service to use.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	ServiceName string `json:"serviceName"`
	// A string map that contains attributes with values that you can use to filter instances by any custom attribute that you specified when you registered the instance
	// +optional
	Attributes []AWSCloudMapInstanceAttribute `json:"attributes,omitempty"`
}

// DNSServiceDiscovery refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_DnsServiceDiscovery.html
type DNSServiceDiscovery struct {
	// Specifies the DNS service discovery hostname for the virtual node.
	Hostname string `json:"hostname"`
}

// ServiceDiscovery refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_ServiceDiscovery.html
type ServiceDiscovery struct {
	// Specifies any AWS Cloud Map information for the virtual node.
	// +optional
	AWSCloudMap *AWSCloudMapServiceDiscovery `json:"awsCloudMap,omitempty"`
	// Specifies the DNS information for the virtual node.
	// +optional
	DNS *DNSServiceDiscovery `json:"dns,omitempty"`
}

// FileAccessLog refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_FileAccessLog.html
type FileAccessLog struct {
	// The file path to write access logs to.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Path string `json:"path"`
}

// AccessLog refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_AccessLog.html
type AccessLog struct {
	// The file object to send virtual node access logs to.
	// +optional
	File *FileAccessLog `json:"file,omitempty"`
}

// Logging refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_Logging.html
type Logging struct {
	// The access log configuration for a virtual node.
	// +optional
	AccessLog *AccessLog `json:"accessLog,omitempty"`
}

type VirtualNodeConditionType string

const (
	// VirtualNodeActive is True when the AppMesh VirtualNode has been created or found via the API
	VirtualNodeActive VirtualNodeConditionType = "VirtualNodeActive"
)

type VirtualNodeCondition struct {
	// Type of VirtualNode condition.
	Type VirtualNodeConditionType `json:"type"`
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

// VirtualNodeSpec defines the desired state of VirtualNode
// refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualNodeSpec.html
type VirtualNodeSpec struct {
	// AWSName is the AppMesh VirtualNode object's name.
	// If unspecified or empty, it defaults to be "${name}_${namespace}" of k8s VirtualNode
	// +optional
	AWSName *string `json:"awsName,omitempty"`
	// PodSelector selects Pods using labels to designate VirtualNode membership.
	// This field follows standard label selector semantics:
	//	if present but empty, it selects all pods within namespace.
	// 	if absent, it selects no pod.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// The listener that the virtual node is expected to receive inbound traffic from
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=1
	// +optional
	Listeners []Listener `json:"listeners,omitempty"`
	// The service discovery information for the virtual node. Optional if there is no
	// inbound traffic(no listeners). Mandatory if a listener is specified.
	// +optional
	ServiceDiscovery *ServiceDiscovery `json:"serviceDiscovery,omitempty"`
	// The backends that the virtual node is expected to send outbound traffic to.
	// +optional
	Backends []Backend `json:"backends,omitempty"`
	// A reference to an object that represents the defaults for backends.
	// +optional
	BackendDefaults *BackendDefaults `json:"backendDefaults,omitempty"`
	// The inbound and outbound access logging information for the virtual node.
	// +optional
	Logging *Logging `json:"logging,omitempty"`

	// A reference to k8s Mesh CR that this VirtualNode belongs to.
	// The admission controller populates it using Meshes's selector, and prevents users from setting this field.
	//
	// Populated by the system.
	// Read-only.
	// +optional
	MeshRef *MeshReference `json:"meshRef,omitempty"`
}

// VirtualNodeStatus defines the observed state of VirtualNode
type VirtualNodeStatus struct {
	// VirtualNodeARN is the AppMesh VirtualNode object's Amazon Resource Name
	// +optional
	VirtualNodeARN *string `json:"virtualNodeARN,omitempty"`
	// The current VirtualNode status.
	// +optional
	Conditions []VirtualNodeCondition `json:"conditions,omitempty"`

	// The generation observed by the VirtualNode controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualNode is the Schema for the virtualnodes API
type VirtualNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualNodeSpec   `json:"spec,omitempty"`
	Status VirtualNodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualNodeList contains a list of VirtualNode
type VirtualNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNode `json:"items"`
}
