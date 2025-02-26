/*
Copyright 2024 The KEDA Authors.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HTTPInterceptorScalingSpec defines the desired state of Interceptor autoscaling
type HTTPInterceptorScalingSpec struct {
	// +kubebuilder:default=3
	// +optional
	// Minimum replicas for the interceptor
	MinReplicas int `json:"minReplicas"`
	// +kubebuilder:default=100
	// +optional
	// Maximum replicas for the interceptor
	MaxReplicas int `json:"maxReplicas"`
	// +kubebuilder:default=100
	// +optional
	// Target concurrent requests
	Target int `json:"target"`
}

// HTTPInterceptorConfigurationSpec defines the desired state of Interceptor configuration
type HTTPInterceptorConfigurationSpec struct {
	// +optional
	// Port to be used for proxy operations
	ProxyPort *int32 `json:"proxyPort,omitempty"`
	// +optional
	// Port to be used for admin operations
	AdminPort *int32 `json:"adminPort,omitempty"`
	// +optional
	// Timeout for establishing the connection
	ConnectTimeout *string `json:"connectTimeout,omitempty"`
	// +optional
	// How long to wait between when the HTTP request
	// is sent to the backing app and when response headers need to arrive
	HeaderTimeout *string `json:"headerTimeout,omitempty"`
	// +optional
	// How long to wait for the backing workload
	// to have 1 or more replicas before connecting and sending the HTTP request.
	WaitTimeout *string `json:"waitTimeout,omitempty"`
	// +optional
	// Timeout after which a connection in the interceptor's
	// internal connection pool will be closed
	IdleConnTimeout *string `json:"idleConnTimeout,omitempty"`
	// +optional
	// Max amount of time the interceptor will
	// wait to establish a TLS connection
	TLSHandshakeTimeout *string `json:"handshakeTimeout,omitempty"`
	// +optional
	// Max amount of time the interceptor will wait
	// after sending request headers if the server returned an Expect: 100-continue
	// header
	ExpectContinueTimeout *string `json:"expectContinueTimeout,omitempty"`
	// +optional
	// Try to force HTTP2 for all requests
	ForceHTTP2 *bool `json:"forceHTTP2,omitempty"`
	// +optional
	// Interval between keepalive probes
	KeepAlive *string `json:"keepAlive,omitempty"`
	// +optional
	// Max number of connections that can be idle in the
	// interceptor's internal connection pool
	MaxIdleConns *int `json:"maxIdleConnections,omitempty"`
	// +optional
	// The interceptor has an internal process that periodically fetches the state
	// of endpoints that is running the servers it forwards to.
	// This is the interval (in milliseconds) representing how often to do a fetch
	PollingInterval *int `json:"pollingInterval,omitempty"`
}

// HTTPInterceptorSpec defines the desired state of Interceptor component
type HTTPInterceptorSpec struct {
	// +optional
	// Traffic configuration
	Config *HTTPInterceptorConfigurationSpec `json:"config,omitempty"`
	// Number of replicas for the interceptor
	Replicas *int32 `json:"replicas,omitempty"`
	// Container image name.
	// +optional
	Image *string `json:"image,omitempty"`
	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Autoscaling options for the interceptor
	Autoscaling *HTTPInterceptorScalingSpec `json:"autoscaling,omitempty"`
	// Compute Resources required by this interceptor.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +kubebuilder:default=default
	// +optional
	// Name of the service account to be used
	ServiceAccountName string `json:"serviceAccountName"`
}

// HTTPScalerConfigurationSpec defines the desired state of scaler configuration
type HTTPScalerConfigurationSpec struct {
	// +kubebuilder:default=9090
	// +optional
	// Port to be used for proxy operations
	Port *int32 `json:"port,omitempty"`
}

// HTTPScalerSpec defines the desired state of Scaler component
type HTTPScalerSpec struct {
	// +kubebuilder:default={}
	// +optional
	// Traffic configuration
	Config HTTPScalerConfigurationSpec `json:"config,omitempty"`
	// Number of replicas for the interceptor
	Replicas *int32 `json:"replicas,omitempty"`
	// Container image name.
	// +optional
	Image *string `json:"image,omitempty"`
	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Compute Resources required by this scaler.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +kubebuilder:default=default
	// +optional
	// Name of the service account to be used
	ServiceAccountName string `json:"serviceAccountName"`
}

// HTTPScalingSetSpec defines the desired state of HTTPScalingSet
type HTTPScalingSetSpec struct {
	Interceptor HTTPInterceptorSpec `json:"interceptor"`
	Scaler      HTTPScalerSpec      `json:"scaler"`
}

// HTTPScalingSetStatus defines the observed state of HTTPScalingSet
type HTTPScalingSetStatus struct{}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=ss
// +kubebuilder:subresource:status

// HTTPScalingSet is the Schema for the httpscalingset API
type HTTPScalingSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HTTPScalingSetSpec   `json:"spec,omitempty"`
	Status HTTPScalingSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// HTTPScalingSetList contains a list of HTTPScalingSetList
type HTTPScalingSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HTTPScalingSet `json:"items"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=css,scope=Cluster
// +kubebuilder:subresource:status

// ClusterHTTPScalingSet is the Schema for the cluster httpscalingset API
type ClusterHTTPScalingSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HTTPScalingSetSpec   `json:"spec,omitempty"`
	Status HTTPScalingSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// ClusterHTTPScalingSetList contains a list of ClusterHTTPScalingSet
type ClusterHTTPScalingSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterHTTPScalingSet `json:"items"`
}
