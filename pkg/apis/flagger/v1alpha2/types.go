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

package v1alpha2

import (
	hpav1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CanaryKind              = "Canary"
	ProgressDeadlineSeconds = 600
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
	// reference to target resource
	TargetRef hpav1.CrossVersionObjectReference `json:"targetRef"`

	// reference to autoscaling resource
	AutoscalerRef hpav1.CrossVersionObjectReference `json:"autoscalerRef"`

	// virtual service spec
	Service CanaryService `json:"service"`

	// metrics and thresholds
	CanaryAnalysis CanaryAnalysis `json:"canaryAnalysis"`

	// the maximum time in seconds for a canary deployment to make progress
	// before it is considered to be failed. Defaults to 60s.
	ProgressDeadlineSeconds *int32 `json:"progressDeadlineSeconds,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CanaryList is a list of Canary resources
type CanaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Canary `json:"items"`
}

// CanaryState used for status state op
type CanaryState string

const (
	CanaryRunning     CanaryState = "running"
	CanaryFinished    CanaryState = "finished"
	CanaryFailed      CanaryState = "failed"
	CanaryInitialized CanaryState = "initialized"
)

// CanaryStatus is used for state persistence (read-only)
type CanaryStatus struct {
	State          CanaryState `json:"state"`
	CanaryRevision string      `json:"canaryRevision"`
	FailedChecks   int         `json:"failedChecks"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// CanaryService is used to create ClusterIP services
// and Istio Virtual Service
type CanaryService struct {
	Port     int32    `json:"port"`
	Gateways []string `json:"gateways"`
	Hosts    []string `json:"hosts"`
}

// CanaryAnalysis is used to describe how the analysis should be done
type CanaryAnalysis struct {
	Threshold  int             `json:"threshold"`
	MaxWeight  int             `json:"maxWeight"`
	StepWeight int             `json:"stepWeight"`
	Metrics    []CanaryMetric  `json:"metrics"`
	Webhooks   []CanaryWebhook `json:"webhooks,omitempty"`
}

// CanaryMetric holds the reference to Istio metrics used for canary analysis
type CanaryMetric struct {
	Name      string `json:"name"`
	Interval  string `json:"interval"`
	Threshold int    `json:"threshold"`
}

// CanaryWebhook holds the reference to external checks used for canary analysis
type CanaryWebhook struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Timeout string `json:"timeout"`
	// +optional
	Metadata *map[string]string `json:"metadata,omitempty"`
}

// CanaryWebhookPayload holds the deployment info and metadata sent to webhooks
type CanaryWebhookPayload struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Metadata  *map[string]string `json:"metadata,omitempty"`
}

// GetProgressDeadlineSeconds returns the progress deadline (default 600s)
func (c *Canary) GetProgressDeadlineSeconds() int {
	if c.Spec.ProgressDeadlineSeconds != nil {
		return int(*c.Spec.ProgressDeadlineSeconds)
	}

	return ProgressDeadlineSeconds
}
