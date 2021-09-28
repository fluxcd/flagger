/*


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

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=ALLOW_ALL;DROP_ALL
type EgressFilterType string

const (
	// EgressFilterTypeAllowAll allows egress to any endpoint inside or outside of the service mesh
	EgressFilterTypeAllowAll EgressFilterType = "ALLOW_ALL"
	// EgressFilterTypeDropAll allows egress only from virtual nodes to other defined resources in the service mesh (and any traffic to *.amazonaws.com for AWS API calls)
	EgressFilterTypeDropAll EgressFilterType = "DROP_ALL"
)

// EgressFilter refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_EgressFilter.html
type EgressFilter struct {
	// The egress filter type.
	Type EgressFilterType `json:"type"`
}

type MeshConditionType string

const (
	// MeshActive is True when the AppMesh Mesh has been created or found via the API
	MeshActive MeshConditionType = "MeshActive"
)

type MeshCondition struct {
	// Type of mesh condition.
	Type MeshConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason *string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message *string `json:"message,omitempty"`
}

// MeshSpec defines the desired state of Mesh
// refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_MeshSpec.html
type MeshSpec struct {
	// AWSName is the AppMesh Mesh object's name.
	// If unspecified or empty, it defaults to be "${name}" of k8s Mesh
	// +optional
	AWSName *string `json:"awsName,omitempty"`
	// NamespaceSelector selects Namespaces using labels to designate mesh membership.
	// This field follows standard label selector semantics:
	//	if present but empty, it selects all namespaces.
	// 	if absent, it selects no namespace.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	// The egress filter rules for the service mesh.
	// If unspecified, default settings from AWS API will be applied. Refer to AWS Docs for default settings.
	// +optional
	EgressFilter *EgressFilter `json:"egressFilter,omitempty"`
	// The AWS IAM account ID of the service mesh owner.
	// Required if the account ID is not your own.
	// +optional
	MeshOwner *string `json:"meshOwner,omitempty"`
}

// MeshStatus defines the observed state of Mesh
type MeshStatus struct {
	// MeshARN is the AppMesh Mesh object's Amazon Resource Name
	// +optional
	MeshARN *string `json:"meshARN,omitempty"`
	// The current Mesh status.
	// +optional
	Conditions []MeshCondition `json:"conditions,omitempty"`

	// The generation observed by the Mesh controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.meshARN",description="The AppMesh Mesh object's Amazon Resource Name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// Mesh is the Schema for the meshes API
type Mesh struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MeshSpec   `json:"spec,omitempty"`
	Status MeshStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MeshList contains a list of Mesh
type MeshList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Mesh `json:"items"`
}

/**
func init() {
	SchemeBuilder.Register(&Mesh{}, &MeshList{})
}
**/
