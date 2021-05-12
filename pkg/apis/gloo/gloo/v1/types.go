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
	SslConfig                   *UpstreamSslConfig    `json:"ssl_config,omitempty"`
	CircuitBreakers             *CircuitBreakerConfig `json:"circuit_breakers,omitempty"`
	ConnectionConfig            *ConnectionConfig     `json:"connection_config,omitempty"`
	UseHttp2                    bool                  `json:"use_http2,omitempty"`
	InitialStreamWindowSize     uint32                `json:"initial_stream_window_size,omitempty"`
	InitialConnectionWindowSize uint32                `json:"initial_connection_window_size,omitempty"`
	HttpProxyHostname           string                `json:"http_proxy_hostname,omitempty"`
}

type KubeUpstream struct {
	ServiceName      string            `json:"service_name,omitempty"`
	ServiceNamespace string            `json:"service_namespace,omitempty"`
	ServicePort      int32             `json:"service_port,omitempty"`
	Selector         map[string]string `json:"selector,omitempty"`
}

type UpstreamSslConfig struct {
	Sni                  string         `json:"sni,omitempty"`
	VerifySubjectAltName []string       `json:"verify_subject_alt_name,omitempty"`
	Parameters           *SslParameters `json:"parameters,omitempty"`
	AlpnProtocols        []string       `json:"alpn_protocols,omitempty"`

	/** SSLSecrets -- only one of these should be set */
	*UpstreamSslConfig_Sds      `json:"sds,omitempty"`
	SecretRef                   *v1.ResourceRef `json:"secretRef,omitempty"`
	*UpstreamSslConfig_SslFiles `json:"sslFiles,omitempty"`
}

// SSLFiles reference paths to certificates which can be read by the proxy off of its local filesystem
type UpstreamSslConfig_SslFiles struct {
	TlsCert string `json:"tls_cert,omitempty"`
	TlsKey  string `json:"tls_key,omitempty"`
	RootCa  string `json:"root_ca,omitempty"`
}

// Use secret discovery service.
type UpstreamSslConfig_Sds struct {
	TargetUri              string `json:"target_uri,omitempty"`
	CertificatesSecretName string `json:"certificates_secret_name,omitempty"`
	ValidationContextName  string `json:"validation_context_name,omitempty"`

	CallCredentials *CallCredentials `json:"call_credentials,omitempty"`
}

type CallCredentials struct {
	FileCredentialSource *CallCredentials_FileCredentialSource `json:"file_credential_source,omitempty"`
	ClusterName          string                                `json:"cluster_name,omitempty"`
}

type CallCredentials_FileCredentialSource struct {
	TokenFileName string `json:"token_file_name,omitempty"`
	Header        string `json:"header,omitempty"`
}

type SslParameters struct {
	MinimumProtocolVersion int32    `json:"minimum_protocol_version,omitempty"`
	MaximumProtocolVersion int32    `json:"maximum_protocol_version,omitempty"`
	CipherSuites           []string `json:"cipher_suites,omitempty"`
	EcdhCurves             []string `json:"ecdh_curves,omitempty"`
}

type CircuitBreakerConfig struct {
	MaxConnections     uint32 `json:"max_connections,omitempty"`
	MaxPendingRequests uint32 `json:"max_pending_requests,omitempty"`
	MaxRequests        uint32 `json:"max_requests,omitempty"`
	MaxRetries         uint32 `json:"max_retries,omitempty"`
}

type ConnectionConfig struct {
	MaxRequestsPerConnection      uint32                                `json:"max_requests_per_connection,omitempty"`
	ConnectTimeout                *Duration                             `json:"connect_timeout,omitempty"`
	TcpKeepalive                  *ConnectionConfig_TcpKeepAlive        `json:"tcp_keepalive,omitempty"`
	PerConnectionBufferLimitBytes uint32                                `json:"per_connection_buffer_limit_bytes,omitempty"`
	CommonHttpProtocolOptions     *ConnectionConfig_HttpProtocolOptions `json:"common_http_protocol_options,omitempty"`
}

type ConnectionConfig_TcpKeepAlive struct {
	KeepaliveProbes   uint32    `json:"keepalive_probes,omitempty"`
	KeepaliveTime     *Duration `json:"keepalive_time,omitempty"`
	KeepaliveInterval *Duration `json:"keepalive_interval,omitempty"`
}

type ConnectionConfig_HttpProtocolOptions struct {
	IdleTimeout                  *Duration `json:"idle_timeout,omitempty"`
	MaxHeadersCount              uint32    `json:"max_headers_count,omitempty"`
	MaxStreamDuration            *Duration `json:"max_stream_duration,omitempty"`
	HeadersWithUnderscoresAction uint32    `json:"headers_with_underscores_action,omitempty"`
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
