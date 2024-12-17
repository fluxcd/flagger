/*
Copyright 2018 The Flagger Authors.

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

package v1beta1

import (
	"fmt"
	"time"

	v1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1"
	"github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1beta1"
	http "github.com/fluxcd/flagger/pkg/apis/http/v1alpha1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	CanaryKind              = "Canary"
	ProgressDeadlineSeconds = 600
	AnalysisInterval        = 60 * time.Second
	PrimaryReadyThreshold   = 100
	CanaryReadyThreshold    = 100
	MetricInterval          = "1m"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Canary is the configuration for a canary release,
// which automatically manages the bootstrap, analysis, traffic shifting,
// promotion or rollback of an app revision
type Canary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CanarySpec   `json:"spec"`
	Status CanaryStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CanaryList is a list of Canary resources
type CanaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Canary `json:"items"`
}

// CanarySpec is the specification of the desired behavior of the Canary
type CanarySpec struct {
	// Provider overwrites the -mesh-provider flag for this particular canary
	// +optional
	Provider string `json:"provider,omitempty"`

	// MetricsServer overwrites the -metrics-server flag for this particular canary
	// +optional
	MetricsServer string `json:"metricsServer,omitempty"`

	// TargetRef references a target resource
	TargetRef LocalObjectReference `json:"targetRef"`

	// AutoscalerRef references an autoscaling resource
	// +optional
	AutoscalerRef *AutoscalerReference `json:"autoscalerRef,omitempty"`

	// Reference to NGINX ingress resource
	// +optional
	IngressRef *LocalObjectReference `json:"ingressRef,omitempty"`

	// Reference to APISIX route resource
	// +optional
	RouteRef *LocalObjectReference `json:"routeRef,omitempty"`

	// Reference to Gloo Upstream resource. Upstream config is copied from
	// the referenced upstream to the upstreams generated by flagger.
	// +optional
	UpstreamRef *CrossNamespaceObjectReference `json:"upstreamRef,omitempty"`

	// Service defines how ClusterIP services, service mesh or ingress routing objects are generated
	Service CanaryService `json:"service"`

	// Analysis defines the validation process of a release
	Analysis *CanaryAnalysis `json:"analysis,omitempty"`

	// Deprecated: replaced by Analysis
	CanaryAnalysis *CanaryAnalysis `json:"canaryAnalysis,omitempty"`

	// ProgressDeadlineSeconds represents the maximum time in seconds for a
	// canary deployment to make progress before it is considered to be failed
	// +optional
	ProgressDeadlineSeconds *int32 `json:"progressDeadlineSeconds,omitempty"`

	// SkipAnalysis promotes the canary without analysing it
	// +optional
	SkipAnalysis bool `json:"skipAnalysis,omitempty"`

	// revert canary mutation on deletion of canary resource
	// +optional
	RevertOnDeletion bool `json:"revertOnDeletion,omitempty"`

	// Suspend, if set to true will suspend the Canary, disabling any canary runs
	// regardless of any changes to its target, services, etc. Note that if the
	// Canary is suspended during an analysis, its paused until the Canary is unsuspended.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// CanaryService defines how ClusterIP services, service mesh or ingress routing objects are generated
type CanaryService struct {
	// Name of the Kubernetes service generated by Flagger
	// Defaults to CanarySpec.TargetRef.Name
	// +optional
	Name string `json:"name,omitempty"`

	// Port of the generated Kubernetes service
	Port int32 `json:"port"`

	// Port name of the generated Kubernetes service
	// Defaults to http
	// +optional
	PortName string `json:"portName,omitempty"`

	// Target port number or name of the generated Kubernetes service
	// Defaults to CanaryService.Port
	// +optional
	TargetPort intstr.IntOrString `json:"targetPort,omitempty"`

	// AppProtocol of the service
	// https://kubernetes.io/docs/concepts/services-networking/service/#application-protocol
	// +optional
	AppProtocol string `json:"appProtocol,omitempty"`

	// PortDiscovery adds all container ports to the generated Kubernetes service
	PortDiscovery bool `json:"portDiscovery"`

	// Timeout of the HTTP or gRPC request
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// Gateways attached to the generated Istio virtual service
	// Defaults to the internal mesh gateway
	// +optional
	Gateways []string `json:"gateways,omitempty"`

	// Gateways that the HTTPRoute needs to attach itself to.
	// Must be specified while using the Gateway API as a provider.
	// +optional
	GatewayRefs []v1beta1.ParentReference `json:"gatewayRefs,omitempty"`

	// Hosts attached to the generated Istio virtual service or Gateway API HTTPRoute.
	// Defaults to the service name
	// +optional
	Hosts []string `json:"hosts,omitempty"`

	// If enabled, Flagger would generate Istio VirtualServices without hosts and gateway,
	// making the service compatible with Istio delegation. Note that pilot env
	// `PILOT_ENABLE_VIRTUAL_SERVICE_DELEGATE` must also be set.
	// +optional
	Delegation bool `json:"delegation,omitempty"`

	// TrafficPolicy attached to the generated Istio destination rules
	// +optional
	TrafficPolicy *istiov1beta1.TrafficPolicy `json:"trafficPolicy,omitempty"`

	// URI match conditions for the generated service
	// +optional
	Match []istiov1beta1.HTTPMatchRequest `json:"match,omitempty"`

	// Rewrite HTTP URIs for the generated service
	// +optional
	Rewrite *HTTPRewrite `json:"rewrite,omitempty"`

	// Retries policy for the generated virtual service
	// +optional
	Retries *istiov1beta1.HTTPRetry `json:"retries,omitempty"`

	// Headers operations for the generated Istio virtual service
	// +optional
	Headers *istiov1beta1.Headers `json:"headers,omitempty"`

	// Mirror specifies the destination for request mirroring.
	// Responses from this destination are dropped.
	Mirror []v1beta1.HTTPRequestMirrorFilter `json:"mirror,omitempty"`

	// Cross-Origin Resource Sharing policy for the generated Istio virtual service
	// +optional
	CorsPolicy *istiov1beta1.CorsPolicy `json:"corsPolicy,omitempty"`

	// Mesh name of the generated App Mesh virtual nodes and virtual service
	// +optional
	MeshName string `json:"meshName,omitempty"`

	// Backends of the generated App Mesh virtual nodes
	// +optional
	Backends []string `json:"backends,omitempty"`

	// Apex is metadata to add to the apex service
	// +optional
	Apex *CustomMetadata `json:"apex,omitempty"`

	// Primary is the metadata to add to the primary service
	// +optional
	Primary *CustomBackend `json:"primary,omitempty"`

	// Canary is the metadata to add to the canary service
	// +optional
	Canary *CustomBackend `json:"canary,omitempty"`
}

// CanaryAnalysis is used to describe how the analysis should be done
type CanaryAnalysis struct {
	// Schedule interval for this canary analysis
	Interval string `json:"interval"`

	// Number of checks to run for A/B Testing and Blue/Green
	// +optional
	Iterations int `json:"iterations,omitempty"`

	// Enable traffic mirroring for Blue/Green
	// +optional
	Mirror bool `json:"mirror,omitempty"`

	// Weight of the traffic to be mirrored in the range of [0, 100].
	// +optional
	MirrorWeight int `json:"mirrorWeight,omitempty"`

	// Max traffic weight routed to canary
	// +optional
	MaxWeight int `json:"maxWeight,omitempty"`

	// Incremental traffic weight step for analysis phase
	// +optional
	StepWeight int `json:"stepWeight,omitempty"`

	// Incremental traffic weight steps for analysis phase
	// +optional
	StepWeights []int `json:"stepWeights,omitempty"`

	// Incremental traffic weight step for promotion phase
	// +optional
	StepWeightPromotion int `json:"stepWeightPromotion,omitempty"`

	// Max number of failed checks before the canary is terminated
	Threshold int `json:"threshold"`

	// Percentage of pods that need to be available to consider primary as ready
	PrimaryReadyThreshold *int `json:"primaryReadyThreshold,omitempty"`

	// Percentage of pods that need to be available to consider canary as ready
	CanaryReadyThreshold *int `json:"canaryReadyThreshold,omitempty"`

	// Alert list for this canary analysis
	Alerts []CanaryAlert `json:"alerts,omitempty"`

	// Metric check list for this canary analysis
	// +optional
	Metrics []CanaryMetric `json:"metrics,omitempty"`

	// Webhook list for this canary  analysis
	// +optional
	Webhooks []CanaryWebhook `json:"webhooks,omitempty"`

	// A/B testing HTTP header match conditions
	// +optional
	Match []istiov1beta1.HTTPMatchRequest `json:"match,omitempty"`

	// SessionAffinity represents the session affinity settings for a canary run.
	// +optional
	SessionAffinity *SessionAffinity `json:"sessionAffinity,omitempty"`
}

type SessionAffinity struct {
	// CookieName is the key that will be used for the session affinity cookie.
	CookieName string `json:"cookieName,omitempty"`
	// MaxAge indicates the number of seconds until the session affinity cookie will expire.
	// ref: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie#attributes
	// The default value is 86,400 seconds, i.e. a day.
	// +optional
	MaxAge int `json:"maxAge,omitempty"`
}

// CanaryMetric holds the reference to metrics used for canary analysis
type CanaryMetric struct {
	// Name of the metric
	Name string `json:"name"`

	// Interval represents the windows size
	Interval string `json:"interval,omitempty"`

	// Deprecated: Max value accepted for this metric (replaced by ThresholdRange)
	Threshold float64 `json:"threshold,omitempty"`

	// Range value accepted for this metric
	// +optional
	ThresholdRange *CanaryThresholdRange `json:"thresholdRange,omitempty"`

	// Deprecated: Prometheus query for this metric (replaced by TemplateRef)
	// +optional
	Query string `json:"query,omitempty"`

	// TemplateRef references a metric template object
	// +optional
	TemplateRef *CrossNamespaceObjectReference `json:"templateRef,omitempty"`

	// TemplateVariables provides a map of key/value pairs that can be used to inject variables into a metric query.
	// +optional
	TemplateVariables map[string]string `json:"templateVariables,omitempty"`
}

// CanaryThresholdRange defines the range used for metrics validation
type CanaryThresholdRange struct {
	// Minimum value
	// +optional
	Min *float64 `json:"min,omitempty"`

	// Maximum value
	// +optional
	Max *float64 `json:"max,omitempty"`
}

// AlertSeverity defines alert filtering based on severity levels
type AlertSeverity string

const (
	SeverityInfo  AlertSeverity = "info"
	SeverityWarn  AlertSeverity = "warn"
	SeverityError AlertSeverity = "error"
)

// CanaryAlert defines an alert for this canary
type CanaryAlert struct {
	// Name of the alert
	Name string `json:"name"`

	// Severity level: info, warn, error (default info)
	Severity AlertSeverity `json:"severity,omitempty"`

	// Alert provider reference
	ProviderRef CrossNamespaceObjectReference `json:"providerRef"`
}

// HookType can be pre, post or during rollout
type HookType string

const (
	// RolloutHook execute webhook during the canary analysis
	RolloutHook HookType = "rollout"
	// PreRolloutHook execute webhook before routing traffic to canary
	PreRolloutHook HookType = "pre-rollout"
	// PostRolloutHook execute webhook after the canary analysis
	PostRolloutHook HookType = "post-rollout"
	// ConfirmRolloutHook halt canary analysis until webhook returns HTTP 200
	ConfirmRolloutHook HookType = "confirm-rollout"
	// ConfirmPromotionHook halt canary promotion until webhook returns HTTP 200
	ConfirmPromotionHook HookType = "confirm-promotion"
	// EventHook dispatches Flagger events to the specified endpoint
	EventHook HookType = "event"
	// RollbackHook rollback canary analysis if webhook returns HTTP 200
	RollbackHook HookType = "rollback"
	// ConfirmTrafficIncreaseHook increases traffic weight if webhook returns HTTP 200
	ConfirmTrafficIncreaseHook = "confirm-traffic-increase"
)

// CanaryWebhook holds the reference to external checks used for canary analysis
type CanaryWebhook struct {
	// Type of this webhook
	Type HookType `json:"type"`

	// Name of this webhook
	Name string `json:"name"`

	// URL address of this webhook
	URL string `json:"url"`

	// MuteAlert mutes all alerts generated from this webhook, if any
	MuteAlert bool `json:"muteAlert"`

	// Request timeout for this webhook
	Timeout string `json:"timeout,omitempty"`

	// Metadata (key-value pairs) for this webhook
	// +optional
	Metadata *map[string]string `json:"metadata,omitempty"`

	// Number of retries for this webhook
	// +optional
	Retries int `json:"retries,omitempty"`

	// Disable TLS verification for this webhook
	// +optional
	DisableTLS bool `json:"disableTLS,omitempty"`
}

// CanaryWebhookPayload holds the deployment info and metadata sent to webhooks
type CanaryWebhookPayload struct {
	// Name of the canary
	Name string `json:"name"`

	// Namespace of the canary
	Namespace string `json:"namespace"`

	// Phase of the canary analysis
	Phase CanaryPhase `json:"phase"`

	// Hash from the TrackedConfigs and LastAppliedSpec of the Canary.
	// Can be used to identify a Canary for a specific configuration of the
	// deployed resources.
	Checksum string `json:"checksum"`

	// Metadata (key-value pairs) for this webhook
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CrossNamespaceObjectReference contains enough information to let you locate the
// typed referenced object at cluster level
type CrossNamespaceObjectReference struct {
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name of the referent
	Name string `json:"name"`

	// Namespace of the referent
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// LocalObjectReference contains enough information to let you locate the typed
// referenced object in the same namespace.
type LocalObjectReference struct {
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name of the referent
	Name string `json:"name"`
}

// CanaryInterceptorProxyService specifies the service if you want to change
// the Canary interceptor proxy service from its default value.
type CanaryInterceptorProxyService struct {
	// Name of the canary interceptor proxy service.
	// Defaults to "keda-http-add-on-interceptor-proxy".
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace of the canary interceptor proxy service.
	// Defaults to "keda".
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ScalingSetRef defines the desired scaling set to be used
type ScalingSetRef struct {
	// Kind of the resource being referred to. Defaults to HTTPScalingSet.
	// +optional
	Kind http.ScalingSetKind `json:"kind,omitempty"`

	// Name of the scaling set
	Name string `json:"name,omitempty"`

	// Namespace of the scaling set
	// Defaults to "keda".
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type AutoscalerReference struct {
	// API version of the scaler
	// +required
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the scaler
	// +required
	Kind string `json:"kind,omitempty"`

	// Name of the scaler
	// +required
	Name string `json:"name"`

	// PrimaryScalerQueries maps a unique id to a query for the primary
	// scaler, if a scaler supports scaling using queries.
	// +optional
	PrimaryScalerQueries map[string]string `json:"primaryScalerQueries"`

	// PrimaryScalerReplicas defines overrides for the primary
	// autoscaler replicas.
	// +optional
	PrimaryScalerReplicas *ScalerReplicas `json:"primaryScalerReplicas,omitempty"`

	// PrimaryScalingSet is the scaling set to be used for the primary
	// scaler, if a scaler supports scaling using queries.
	// +optional
	PrimaryScalingSet *ScalingSetRef `json:"primaryScalingSet,omitempty"`
}

// ScalerReplicas holds overrides for autoscaler replicas
type ScalerReplicas struct {
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

// CustomMetadata holds labels and annotations to set on generated objects.
type CustomMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// CustomBackend holds labels, annotations, and proxyRef to set on generated objects.
type CustomBackend struct {
	CustomMetadata

	// Ref references a Kubernetes object.
	BackendObjectReference *v1.BackendObjectReference `json:"backendRef,omitempty"`

	// Filters defined at this level should be executed if and only if the
	// request is being forwarded to the backend defined here.
	//
	// Support: Implementation-specific (For broader support of filters, use the
	// Filters field in HTTPRouteRule.)
	//
	// +optional
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:message="May specify either httpRouteFilterRequestRedirect or httpRouteFilterRequestRewrite, but not both",rule="!(self.exists(f, f.type == 'RequestRedirect') && self.exists(f, f.type == 'URLRewrite'))"
	// +kubebuilder:validation:XValidation:message="May specify either httpRouteFilterRequestRedirect or httpRouteFilterRequestRewrite, but not both",rule="!(self.exists(f, f.type == 'RequestRedirect') && self.exists(f, f.type == 'URLRewrite'))"
	// +kubebuilder:validation:XValidation:message="RequestHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'RequestHeaderModifier').size() <= 1"
	// +kubebuilder:validation:XValidation:message="ResponseHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'ResponseHeaderModifier').size() <= 1"
	// +kubebuilder:validation:XValidation:message="RequestRedirect filter cannot be repeated",rule="self.filter(f, f.type == 'RequestRedirect').size() <= 1"
	// +kubebuilder:validation:XValidation:message="URLRewrite filter cannot be repeated",rule="self.filter(f, f.type == 'URLRewrite').size() <= 1"
	Filters []v1.HTTPRouteFilter `json:"filters,omitempty"`
}

// HTTPRewrite holds information about how to modify a request URI during
// forwarding.
type HTTPRewrite struct {
	// rewrite the path (or the prefix) portion of the URI with this
	// value. If the original URI was matched based on prefix, the value
	// provided in this field will replace the corresponding matched prefix.
	Uri string `json:"uri,omitempty"`

	// rewrite the Authority/Host header with this value.
	Authority string `json:"authority,omitempty"`

	// Type is the type of path modification to make.
	// +optional
	Type string `json:"type,omitempty"`
}

// GetType returns the type of HTTP path rewrite to be performed.
func (r *HTTPRewrite) GetType() string {
	if r.Type == string(v1beta1.PrefixMatchHTTPPathModifier) {
		return r.Type
	}
	return string(v1beta1.FullPathHTTPPathModifier)
}

// GetIstioRewrite returns a istiov1beta1.HTTPRewrite object.
func (s *CanaryService) GetIstioRewrite() *istiov1beta1.HTTPRewrite {
	if s.Rewrite != nil {
		return &istiov1beta1.HTTPRewrite{
			Authority: s.Rewrite.Authority,
			Uri:       s.Rewrite.Uri,
		}
	}
	return nil
}

// GetMaxAge returns the max age of a cookie in seconds.
func (s *SessionAffinity) GetMaxAge() int {
	if s.MaxAge == 0 {
		// 24 hours * 60 mins * 60 seconds
		return 86400
	}
	return s.MaxAge
}

// GetServiceNames returns the apex, primary and canary Kubernetes service names
func (c *Canary) GetServiceNames() (apexName, primaryName, canaryName string) {
	apexName = c.Spec.TargetRef.Name
	if c.Spec.Service.Name != "" {
		apexName = c.Spec.Service.Name
	}
	primaryName = fmt.Sprintf("%s-primary", apexName)
	canaryName = fmt.Sprintf("%s-canary", apexName)
	return
}

// GetProgressDeadlineSeconds returns the progress deadline (default 600s)
func (c *Canary) GetProgressDeadlineSeconds() int {
	if c.Spec.ProgressDeadlineSeconds != nil {
		return int(*c.Spec.ProgressDeadlineSeconds)
	}

	return ProgressDeadlineSeconds
}

// GetAnalysis returns the analysis v1beta1 or v1alpha3
// to be removed along with spec.canaryAnalysis in v1
func (c *Canary) GetAnalysis() *CanaryAnalysis {
	if c.Spec.Analysis != nil {
		return c.Spec.Analysis
	}
	return c.Spec.CanaryAnalysis
}

// GetAnalysisInterval returns the canary analysis interval (default 60s)
func (c *Canary) GetAnalysisInterval() time.Duration {
	if c.GetAnalysis().Interval == "" {
		return AnalysisInterval
	}

	interval, err := time.ParseDuration(c.GetAnalysis().Interval)
	if err != nil {
		return AnalysisInterval
	}

	if interval < 10*time.Second {
		return time.Second * 10
	}

	return interval
}

// GetAnalysisThreshold returns the canary threshold (default 1)
func (c *Canary) GetAnalysisThreshold() int {
	if c.GetAnalysis().Threshold > 0 {
		return c.GetAnalysis().Threshold
	}
	return 1
}

// GetAnalysisPrimaryReadyThreshold returns the canary primaryReadyThreshold (default 100)
func (c *Canary) GetAnalysisPrimaryReadyThreshold() int {
	if c.GetAnalysis().PrimaryReadyThreshold != nil {
		return *c.GetAnalysis().PrimaryReadyThreshold
	}
	return PrimaryReadyThreshold
}

// GetAnalysisCanaryReadyThreshold returns the canary canaryReadyThreshold (default 100)
func (c *Canary) GetAnalysisCanaryReadyThreshold() int {
	if c.GetAnalysis().CanaryReadyThreshold != nil {
		return *c.GetAnalysis().CanaryReadyThreshold
	}
	return CanaryReadyThreshold
}

// GetMetricInterval returns the metric interval default value (1m)
func (c *Canary) GetMetricInterval() string {
	return MetricInterval
}

// SkipAnalysis returns true if the analysis is nil
// or if spec.SkipAnalysis is true
func (c *Canary) SkipAnalysis() bool {
	if c.Spec.Analysis == nil && c.Spec.CanaryAnalysis == nil {
		return true
	}
	return c.Spec.SkipAnalysis
}

// IsHTTPScaledObject returns true if the autoscalerRef is a HTTPScaledObject
func (c *Canary) IsHTTPScaledObject() bool {
	return c.Spec.AutoscalerRef != nil && c.Spec.AutoscalerRef.Kind == "HTTPScaledObject"
}
