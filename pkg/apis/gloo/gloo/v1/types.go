package v1

import (
	v1 "github.com/fluxcd/flagger/pkg/apis/gloo/gateway/v1"
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
	Kube                        *KubeUpstream         `json:"kube,omitempty"`
	Labels                      map[string]string     `json:"labels,omitempty"`
	SslConfig                   *UpstreamSslConfig    `json:"sslConfig,omitempty"`
	CircuitBreakers             *CircuitBreakerConfig `json:"circuitBreakers,omitempty"`
	ConnectionConfig            *ConnectionConfig     `json:"connectionConfig,omitempty"`
	LoadBalancerConfig          *LoadBalancerConfig   `json:"loadBalancerConfig,omitempty"`
	UseHttp2                    bool                  `json:"useHttp2,omitempty"`
	InitialStreamWindowSize     uint32                `json:"initialStreamWindowSize,omitempty"`
	InitialConnectionWindowSize uint32                `json:"initialConnectionWindowSize,omitempty"`
	HttpProxyHostname           string                `json:"httpProxyHostName,omitempty"`
}

type KubeUpstream struct {
	ServiceName      string            `json:"serviceName,omitempty"`
	ServiceNamespace string            `json:"serviceNamespace,omitempty"`
	ServicePort      int32             `json:"servicePort,omitempty"`
	Selector         map[string]string `json:"selector,omitempty"`
}

type UpstreamSslConfig struct {
	Sni                  string         `json:"sni,omitempty"`
	VerifySubjectAltName []string       `json:"verifySubjectAltName,omitempty"`
	Parameters           *SslParameters `json:"parameters,omitempty"`
	AlpnProtocols        []string       `json:"alpnProtocols,omitempty"`

	/** SSLSecrets -- only one of these should be set */
	*UpstreamSslConfig_Sds      `json:"sds,omitempty"`
	SecretRef                   *v1.ResourceRef `json:"secretRef,omitempty"`
	*UpstreamSslConfig_SslFiles `json:"sslFiles,omitempty"`
}

// SSLFiles reference paths to certificates which can be read by the proxy off of its local filesystem
type UpstreamSslConfig_SslFiles struct {
	TlsCert string `json:"tlsCert,omitempty"`
	TlsKey  string `json:"tlsKey,omitempty"`
	RootCa  string `json:"rootCa,omitempty"`
}

// Use secret discovery service.
type UpstreamSslConfig_Sds struct {
	TargetUri              string `json:"targetUri,omitempty"`
	CertificatesSecretName string `json:"certificatesSecretName,omitempty"`
	ValidationContextName  string `json:"validationContextName,omitempty"`

	/** SDSBuilder -- onle one of the following can be set */
	CallCredentials *CallCredentials `json:"callCredentials,omitempty"`
	ClusterName     string           `json:"clusterName,omitempty"`
}

type CallCredentials struct {
	FileCredentialSource *CallCredentials_FileCredentialSource `json:"fileCredentialSource,omitempty"`
}

type CallCredentials_FileCredentialSource struct {
	TokenFileName string `json:"tokenFileName,omitempty"`
	Header        string `json:"header,omitempty"`
}

type SslParameters struct {
	MinimumProtocolVersion int32    `json:"minimumProtocolVersion,omitempty"`
	MaximumProtocolVersion int32    `json:"maximumProtocolVersion,omitempty"`
	CipherSuites           []string `json:"cipherSuites,omitempty"`
	EcdhCurves             []string `json:"ecdhCurves,omitempty"`
}

type CircuitBreakerConfig struct {
	MaxConnections     uint32 `json:"maxConnections,omitempty"`
	MaxPendingRequests uint32 `json:"maxPendingRequests,omitempty"`
	MaxRequests        uint32 `json:"maxRequests,omitempty"`
	MaxRetries         uint32 `json:"maxRetries,omitempty"`
}

type ConnectionConfig struct {
	MaxRequestsPerConnection      uint32                                `json:"maxRequestsPerConnection,omitempty"`
	ConnectTimeout                *Duration                             `json:"connectTimeout,omitempty"`
	TcpKeepalive                  *ConnectionConfig_TcpKeepAlive        `json:"tcpKeepalive,omitempty"`
	PerConnectionBufferLimitBytes uint32                                `json:"perConnectionBufferLimitBytes,omitempty"`
	CommonHttpProtocolOptions     *ConnectionConfig_HttpProtocolOptions `json:"commonHttpProtocolOptions,omitempty"`
}

type ConnectionConfig_TcpKeepAlive struct {
	KeepaliveProbes   uint32    `json:"keepaliveProbes,omitempty"`
	KeepaliveTime     *Duration `json:"keepaliveTime,omitempty"`
	KeepaliveInterval *Duration `json:"keepaliveInterval,omitempty"`
}

type ConnectionConfig_HttpProtocolOptions struct {
	IdleTimeout                  *Duration `json:"idleTimeout,omitempty"`
	MaxHeadersCount              uint32    `json:"maxHeadersCount,omitempty"`
	MaxStreamDuration            *Duration `json:"maxStreamDuration,omitempty"`
	HeadersWithUnderscoresAction uint32    `json:"headersWithUnderscoresAction,omitempty"`
}

type LoadBalancerConfig struct {
	RoundRobin   *LoadBalancerConfigRoundRobin   `json:"roundRobin,omitempty"`
	LeastRequest *LoadBalancerConfigLeastRequest `json:"leastRequest,omitempty"`
}

type LoadBalancerConfigRoundRobin struct {
	SlowStartConfig *SlowStartConfig `json:"slowStartConfig,omitempty"`
}

type LoadBalancerConfigLeastRequest struct {
	SlowStartConfig *SlowStartConfig `json:"slowStartConfig,omitempty"`
	ChoiceCount     uint32           `json:"choiceCount,omitempty"`
}

type SlowStartConfig struct {
	SlowStartWindow  string  `json:"slowStartWindow,omitempty"`
	Aggression       float64 `json:"aggression,omitempty"`
	MinWeightPercent float64 `json:"minWeightPercent,omitempty"`
}

type Duration struct {
	Seconds int64 `json:"seconds,omitempty"`
	Nanos   int32 `json:"nanos,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpstreamList is a list of Upstream resources
type UpstreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Upstream `json:"items"`
}
