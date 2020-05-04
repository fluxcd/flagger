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
	// The VirtualService that is acting as a virtual node backend.
	VirtualServiceRef VirtualServiceReference `json:"virtualServiceRef"`
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
	// If unspecified, defaults to be 10
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	HealthyThreshold *int64 `json:"healthyThreshold,omitempty"`
	// The time period in milliseconds between each health check execution.
	// If unspecified, defaults to be 30000
	// +kubebuilder:validation:Minimum=5000
	// +kubebuilder:validation:Maximum=300000
	// +optional
	IntervalMillis *int64 `json:"intervalMillis,omitempty"`
	// The destination path for the health check request.
	// This value is only used if the specified protocol is http or http2. For any other protocol, this value is ignored.
	// +optional
	Path *string `json:"path,omitempty"`
	// The destination port for the health check request.
	// If unspecified, defaults to be same as port defined in the PortMapping for the listener.
	// +optional
	Port *PortNumber `json:"port,omitempty"`
	// The protocol for the health check request
	// If unspecified, defaults to be same as protocol defined in the PortMapping for the listener.
	// +optional
	Protocol *PortProtocol `json:"protocol,omitempty"`
	// The amount of time to wait when receiving a response from the health check, in milliseconds.
	// If unspecified, defaults to be 5000
	// +kubebuilder:validation:Minimum=2000
	// +kubebuilder:validation:Maximum=60000
	// +optional
	TimeoutMillis *int64 `json:"timeoutMillis,omitempty"`
	// The number of consecutive failed health checks that must occur before declaring a virtual node unhealthy.
	// If unspecified, defaults to be 2
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	UnhealthyThreshold *int64 `json:"unhealthyThreshold,omitempty"`
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

// Listener refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_Listener.html
type Listener struct {
	// The port mapping information for the listener.
	PortMapping PortMapping `json:"portMapping"`
	// The health check information for the listener.
	// +optional
	HealthCheck *HealthCheckPolicy `json:"healthCheck,omitempty"`
	// A reference to an object that represents the Transport Layer Security (TLS) properties for a listener.
	// +optional
	TLS *ListenerTLS `json:"tls,omitempty"`
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

// AWSCloudMapServiceStatus is AWS CloudMap Service object's info
type AWSCloudMapServiceStatus struct {
	// NamespaceID is AWS CloudMap Service object's namespace Id
	// +optional
	NamespaceID *string `json:"namespaceID,omitempty"`
	// ServiceID is AWS CloudMap Service object's Id
	// +optional
	ServiceID *string `json:"serviceID,omitempty"`
}

// VirtualNodeSpec defines the desired state of VirtualNode
// refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualServiceSpec.html
type VirtualNodeSpec struct {
	// AWSName is the AppMesh VirtualNode object's name.
	// If unspecified or empty, it defaults to be "${name}_${namespace}" of k8s VirtualNode
	// +optional
	AWSName *string `json:"awsName,omitempty"`
	// PodSelector selects Pods using labels to designate VirtualNode membership.
	// if unspecified or empty, it selects no pods.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// The listener that the virtual node is expected to receive inbound traffic from
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=1
	// +optional
	Listeners []Listener `json:"listeners,omitempty"`
	// The service discovery information for the virtual node.
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
	// AWSCloudMapServiceStatus is AWS CloudMap Service object's info
	// +optional
	AWSCloudMapServiceStatus *AWSCloudMapServiceStatus `json:"awsCloudMapServiceStatus,omitempty"`
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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualNodeList contains a list of VirtualNode
type VirtualNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNode `json:"items"`
}
