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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AlertProviderKind = "AlertProvider"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AlertProvider is the configuration of alerting for a specific provider
type AlertProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AlertProviderSpec   `json:"spec"`
	Status AlertProviderStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AlertProviderList is a list of alert provider resources
type AlertProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []AlertProvider `json:"items"`
}

// AlertProviderSpec is the specification of the desired behavior of the AlertProvider
type AlertProviderSpec struct {
	// Type of provider
	Type string `json:"type"`

	// Alert channel for this provider
	// +optional
	Channel string `json:"channel,omitempty"`

	// Bot username for this provider
	// +optional
	Username string `json:"username,omitempty"`

	// HTTP(S) webhook address of this provider
	// +optional
	Address string `json:"address,omitempty"`

	// HTTP/S address of the proxy
	// +optional
	Proxy string `json:"proxy,omitempty"`

	// Secret reference containing the provider webhook URL
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

type AlertProviderStatus struct {
	// Conditions of this status
	Conditions []AlertProviderCondition `json:"conditions,omitempty"`
}

type AlertProviderCondition struct {
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
