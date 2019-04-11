package v1

import (
	"encoding/json"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Resource is the generic Kubernetes API object wrapper for Gloo Resources
type Resource struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            core.Status `json:"status"`
	Spec              *Spec       `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceList is the generic Kubernetes API object wrapper
type ResourceList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata"`
	Items           []Resource `json:"items"`
}

// spec implements deepcopy
type Spec map[string]interface{}

func (in *Resource) GetObjectKind() schema.ObjectKind {
	t := in.TypeMeta
	return &t
}

func (in *Spec) DeepCopyInto(out *Spec) {
	if in == nil {
		out = nil
		return
	}
	data, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(data, &out)
	if err != nil {
		panic(err)
	}
}
