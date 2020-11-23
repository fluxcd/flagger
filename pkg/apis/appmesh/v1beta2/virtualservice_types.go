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

// VirtualNodeServiceProvider refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualNodeServiceProvider.html
type VirtualNodeServiceProvider struct {
	// Reference to Kubernetes VirtualNode CR in cluster that is acting as a service provider. Exactly one of 'virtualNodeRef' or 'virtualNodeARN' must be specified.
	// +optional
	VirtualNodeRef *VirtualNodeReference `json:"virtualNodeRef,omitempty"`
	// Amazon Resource Name to AppMesh VirtualNode object that is acting as a service provider. Exactly one of 'virtualNodeRef' or 'virtualNodeARN' must be specified.
	// +optional
	VirtualNodeARN *string `json:"virtualNodeARN,omitempty"`
}

// VirtualRouterServiceProvider refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualRouterServiceProvider.html
type VirtualRouterServiceProvider struct {
	// Reference to Kubernetes VirtualRouter CR in cluster that is acting as a service provider. Exactly one of 'virtualRouterRef' or 'virtualRouterARN' must be specified.
	// +optional
	VirtualRouterRef *VirtualRouterReference `json:"virtualRouterRef,omitempty"`
	// Amazon Resource Name to AppMesh VirtualRouter object that is acting as a service provider. Exactly one of 'virtualRouterRef' or 'virtualRouterARN' must be specified.
	// +optional
	VirtualRouterARN *string `json:"virtualRouterARN,omitempty"`
}

// VirtualServiceProvider refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualServiceProvider.html
type VirtualServiceProvider struct {
	// The virtual node associated with a virtual service.
	// +optional
	VirtualNode *VirtualNodeServiceProvider `json:"virtualNode,omitempty"`
	// The virtual router associated with a virtual service.
	// +optional
	VirtualRouter *VirtualRouterServiceProvider `json:"virtualRouter,omitempty"`
}

type VirtualServiceConditionType string

const (
	// VirtualServiceActive is True when the AppMesh VirtualService has been created or found via the API
	VirtualServiceActive VirtualServiceConditionType = "VirtualServiceActive"
)

type VirtualServiceCondition struct {
	// Type of VirtualService condition.
	Type VirtualServiceConditionType `json:"type"`
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

// VirtualServiceSpec defines the desired state of VirtualService
// refers to https://docs.aws.amazon.com/app-mesh/latest/APIReference/API_VirtualServiceSpec.html
type VirtualServiceSpec struct {
	// AWSName is the AppMesh VirtualService object's name.
	// If unspecified or empty, it defaults to be "${name}.${namespace}" of k8s VirtualService
	// +optional
	AWSName *string `json:"awsName,omitempty"`

	// The provider for virtual services. You can specify a single virtual node or virtual router.
	// +optional
	Provider *VirtualServiceProvider `json:"provider,omitempty"`

	// A reference to k8s Mesh CR that this VirtualService belongs to.
	// The admission controller populates it using Meshes's selector, and prevents users from setting this field.
	//
	// Populated by the system.
	// Read-only.
	// +optional
	MeshRef *MeshReference `json:"meshRef,omitempty"`
}

// VirtualServiceStatus defines the observed state of VirtualService
type VirtualServiceStatus struct {
	// VirtualServiceARN is the AppMesh VirtualService object's Amazon Resource Name.
	// +optional
	VirtualServiceARN *string `json:"virtualServiceARN,omitempty"`
	// The current VirtualService status.
	// +optional
	Conditions []VirtualServiceCondition `json:"conditions,omitempty"`

	// The generation observed by the VirtualService controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualService is the Schema for the virtualservices API
type VirtualService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualServiceSpec   `json:"spec,omitempty"`
	Status VirtualServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualServiceList contains a list of VirtualService
type VirtualServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualService `json:"items"`
}
