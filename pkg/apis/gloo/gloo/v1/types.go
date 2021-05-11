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
    Kube      KubeUpstream       `json:"kube,omitempty"`
    SslConfig *UpstreamSslConfig `json:"ssl_config,omitempty"`
}

type KubeUpstream struct {
    ServiceName      string            `json:"service_name,omitempty"`
    ServiceNamespace string            `json:"service_namespace,omitempty"`
    ServicePort      int32             `json:"service_port,omitempty"`
    Selector         map[string]string `json:"selector,omitempty"`
}

type UpstreamSslConfig struct {
    SslSecrets isUpstreamSslConfig_SslSecrets
    Sni                  string         `json:"sni,omitempty"`
    VerifySubjectAltName []string       `json:"verify_subject_alt_name,omitempty"`
    Parameters           *SslParameters `json:"parameters,omitempty"`
    AlpnProtocols        []string       `json:"alpn_protocols,omitempty"`
}

type isUpstreamSslConfig_SslSecrets interface{
    isUpstreamSslConfig_SslSecrets()
}

type UpstreamSslConfig_SecretRef struct {
    SecretRef *v1.ResourceRef
}

// SSLFiles reference paths to certificates which can be read by the proxy off of its local filesystem
type UpstreamSslConfig_SslFiles struct {
    TlsCert string `json:"tls_cert,omitempty"`
    TlsKey string `json:"tls_key,omitempty"`
    RootCa string `json:"root_ca,omitempty"`
}

// Use secret discovery service.
type UpstreamSslConfig_Sds struct {
    TargetUri string `json:"target_uri,omitempty"`
    CertifactesSecretName string `json:"certificates_secret_name,omitempty"`
    ValidationContextName string `json:"validation_context_name,omitempty"`
    SdsBuilder isSDSConfig_SdsBuilder
}

func (*UpstreamSslConfig_SecretRef) isUpstreamSslConfig_SslSecrets() {}
func (*UpstreamSslConfig_SslFiles) isUpstreamSslConfig_SslSecrets() {}
func (*UpstreamSslConfig_Sds) isUpstreamSslConfig_SslSecrets() {}


type isSDSConfig_SdsBuilder interface{
    isSDSConfig_SdsBuilder()
}

type SDSConfig_CallCredentials struct {
    FileCredentialSource *CallCredentials_FileCredentialSource `json:"file_credential_source,omitempty"`
}

type SDSConfig_ClusterName struct {
    ClusterName string `json:cluster_name,omitempty`
}

func (*SDSConfig_CallCredentials) isSDSConfig_SdsBuilder() {}
func (*SDSConfig_ClusterName) isSdsConfig_SdsBuilder() {}

type CallCredentials_FileCredentialSource struct {
    TokenFileName string `json:"token_file_name,omitempty"`
    Header string `json:"header,omitempty"`
}

type SslParameters struct {
    MinimumProtocolVersion int32    `json:"minimum_protocol_version,omitempty"`
    MaximumProtocolVersion int32    `json:"maximum_protocol_version,omitempty"`
    CipherSuites           []string `json:"cipher_suites,omitempty"`
    EcdhCurves             []string `json:"ecdh_curves,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UpstreamList is a list of Upstream resources
type UpstreamList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata"`

    Items []Upstream `json:"items"`
}
