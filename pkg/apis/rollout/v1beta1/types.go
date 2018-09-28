/*
Copyright 2017 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Rollout is a specification for a Rollout resource
type Rollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutSpec   `json:"spec"`
	Status RolloutStatus `json:"status"`
}

// RolloutSpec is the spec for a Rollout resource
type RolloutSpec struct {
	TargetKind     string         `json:"targetKind"`
	Primary        Target         `json:"primary"`
	Canary         Target         `json:"canary"`
	VirtualService VirtualService `json:"virtualService"`
	Metrics        []Metric       `json:"metrics"`
}

type Target struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

type VirtualService struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

type Metric struct {
	Name      string `json:"name"`
	Interval  string `json:"interval"`
	Threshold int    `json:"threshold"`
}

// RolloutStatus is the status for a Rollout resource
type RolloutStatus struct {
	State          string `json:"state"`
	CanaryRevision string `json:"canaryRevision"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RolloutList is a list of Rollout resources
type RolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Rollout `json:"items"`
}
