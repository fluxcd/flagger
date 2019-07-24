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

package v1alpha3

import (
	"time"

	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
	hpav1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CanaryKind              = "Canary"
	ProgressDeadlineSeconds = 600
	AnalysisInterval        = 60 * time.Second
	MetricInterval          = "1m"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Canary is a specification for a Canary resource
type Canary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CanarySpec   `json:"spec"`
	Status CanaryStatus `json:"status"`
}

// CanarySpec is the spec for a Canary resource
type CanarySpec struct {
	// if specified overwrites the -mesh-provider flag for this particular canary
	// +optional
	Provider string `json:"provider,omitempty"`

	// reference to target resource
	TargetRef hpav1.CrossVersionObjectReference `json:"targetRef"`

	// reference to autoscaling resource
	// +optional
	AutoscalerRef *hpav1.CrossVersionObjectReference `json:"autoscalerRef,omitempty"`

	// reference to NGINX ingress resource
	// +optional
	IngressRef *hpav1.CrossVersionObjectReference `json:"ingressRef,omitempty"`

	// virtual service spec
	Service CanaryService `json:"service"`

	// metrics and thresholds
	CanaryAnalysis CanaryAnalysis `json:"canaryAnalysis"`

	// the maximum time in seconds for a canary deployment to make progress
	// before it is considered to be failed. Defaults to ten minutes.
	ProgressDeadlineSeconds *int32 `json:"progressDeadlineSeconds,omitempty"`

	// promote the canary without analysing it
	// +optional
	SkipAnalysis bool `json:"skipAnalysis,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CanaryList is a list of Canary resources
type CanaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Canary `json:"items"`
}

// CanaryService is used to create ClusterIP services
// and Istio Virtual Service
type CanaryService struct {
	Port          int32  `json:"port"`
	PortName      string `json:"portName,omitempty"`
	PortDiscovery bool   `json:"portDiscovery"`
	Timeout       string `json:"timeout,omitempty"`
	// Istio
	Gateways      []string                         `json:"gateways,omitempty"`
	Hosts         []string                         `json:"hosts,omitempty"`
	TrafficPolicy *istiov1alpha3.TrafficPolicy     `json:"trafficPolicy,omitempty"`
	Match         []istiov1alpha3.HTTPMatchRequest `json:"match,omitempty"`
	Rewrite       *istiov1alpha3.HTTPRewrite       `json:"rewrite,omitempty"`
	Retries       *istiov1alpha3.HTTPRetry         `json:"retries,omitempty"`
	Headers       *istiov1alpha3.Headers           `json:"headers,omitempty"`
	CorsPolicy    *istiov1alpha3.CorsPolicy        `json:"corsPolicy,omitempty"`
	// App Mesh
	MeshName string   `json:"meshName,omitempty"`
	Backends []string `json:"backends,omitempty"`
}

// CanaryAnalysis is used to describe how the analysis should be done
type CanaryAnalysis struct {
	Interval   string                           `json:"interval"`
	Threshold  int                              `json:"threshold"`
	MaxWeight  int                              `json:"maxWeight"`
	StepWeight int                              `json:"stepWeight"`
	Metrics    []CanaryMetric                   `json:"metrics"`
	Webhooks   []CanaryWebhook                  `json:"webhooks,omitempty"`
	Match      []istiov1alpha3.HTTPMatchRequest `json:"match,omitempty"`
	Iterations int                              `json:"iterations,omitempty"`
}

// CanaryMetric holds the reference to Istio metrics used for canary analysis
type CanaryMetric struct {
	Name      string  `json:"name"`
	Interval  string  `json:"interval,omitempty"`
	Threshold float64 `json:"threshold"`
	// +optional
	Query string `json:"query,omitempty"`
}

// HookType can be pre, post or during rollout
type HookType string

const (
	// RolloutHook execute webhook during the canary analysis
	RolloutHook HookType = "rollout"
	// PreRolloutHook execute webhook before routing traffic to canary
	PreRolloutHook HookType = "pre-rollout"
	// PreRolloutHook execute webhook after the canary analysis
	PostRolloutHook HookType = "post-rollout"
	// ConfirmRolloutHook halt canary analysis until webhook returns HTTP 200
	ConfirmRolloutHook HookType = "confirm-rollout"
)

// CanaryWebhook holds the reference to external checks used for canary analysis
type CanaryWebhook struct {
	Type    HookType `json:"type"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Timeout string   `json:"timeout"`
	// +optional
	Metadata *map[string]string `json:"metadata,omitempty"`
}

// CanaryWebhookPayload holds the deployment info and metadata sent to webhooks
type CanaryWebhookPayload struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Phase     CanaryPhase       `json:"phase"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// GetProgressDeadlineSeconds returns the progress deadline (default 600s)
func (c *Canary) GetProgressDeadlineSeconds() int {
	if c.Spec.ProgressDeadlineSeconds != nil {
		return int(*c.Spec.ProgressDeadlineSeconds)
	}

	return ProgressDeadlineSeconds
}

// GetAnalysisInterval returns the canary analysis interval (default 60s)
func (c *Canary) GetAnalysisInterval() time.Duration {
	if c.Spec.CanaryAnalysis.Interval == "" {
		return AnalysisInterval
	}

	interval, err := time.ParseDuration(c.Spec.CanaryAnalysis.Interval)
	if err != nil {
		return AnalysisInterval
	}

	if interval < 10*time.Second {
		return time.Second * 10
	}

	return interval
}

// GetMetricInterval returns the metric interval default value (1m)
func (c *Canary) GetMetricInterval() string {
	return MetricInterval
}
