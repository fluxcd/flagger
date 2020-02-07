/*
Copyright The Flagger Authors.

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
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MetricTemplateKind = "MetricTemplate"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MetricTemplate is a specification for a canary analysis metric
type MetricTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetricTemplateSpec   `json:"spec"`
	Status MetricTemplateStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MetricTemplateList is a list of metric template resources
type MetricTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MetricTemplate `json:"items"`
}

// MetricTemplateSpec is the spec for a metric template resource
type MetricTemplateSpec struct {
	// Provider of this metric
	Provider MetricTemplateProvider `json:"provider,omitempty"`

	// Query template for this metric
	Query string `json:"query,omitempty"`
}

// MetricProvider is the spec for a MetricProvider resource
type MetricTemplateProvider struct {
	// Type of provider
	Type string `json:"type,omitempty"`

	// HTTP(S) address of this provider
	Address string `json:"address,omitempty"`

	// Secret reference containing the provider credentials
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// MetricTemplateModel is the query template model
type MetricTemplateModel struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Target    string `json:"target"`
	Service   string `json:"service"`
	Ingress   string `json:"ingress"`
	Interval  string `json:"interval"`
}

// TemplateFunctions returns a map of functions, one for each model field
func (mtm *MetricTemplateModel) TemplateFunctions() template.FuncMap {
	return template.FuncMap{
		"name":      func() string { return mtm.Name },
		"namespace": func() string { return mtm.Namespace },
		"target":    func() string { return mtm.Target },
		"service":   func() string { return mtm.Service },
		"ingress":   func() string { return mtm.Ingress },
		"interval":  func() string { return mtm.Interval },
	}
}

type MetricTemplateStatus struct {
	// Conditions of this status
	Conditions []MetricTemplateCondition `json:"conditions,omitempty"`
}

type MetricTemplateCondition struct {
	// Type of this condition
	Type string `json:"type"`

	// Status of this condition
	Status corev1.ConditionStatus `json:"status"`

	// LastUpdateTime of this condition
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastTransitionTime of this condition
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason for the current status of this condition
	Reason string `json:"reason,omitempty"`

	// Message associated with this condition
	Message string `json:"message,omitempty"`
}
