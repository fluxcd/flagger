package crd

import (
	"fmt"
	"log"
	"sync"

	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/client/clientset/versioned/scheme"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TODO(ilackarms): evaluate this fix for concurrent map access in k8s.io/apimachinery/pkg/runtime.SchemaBuider
var registerLock sync.Mutex

type Crd struct {
	GroupName     string
	Plural        string
	Group         string
	Version       string
	KindName      string
	ShortName     string
	ClusterScoped bool
	Type          runtime.Object
}

func NewCrd(GroupName string,
	Plural string,
	Group string,
	Version string,
	KindName string,
	ShortName string,
	ClusterScoped bool,
	Type runtime.Object) Crd {
	c := Crd{
		GroupName:     GroupName,
		Plural:        Plural,
		Group:         Group,
		Version:       Version,
		KindName:      KindName,
		ShortName:     ShortName,
		ClusterScoped: ClusterScoped,
		Type:          Type,
	}
	if err := c.AddToScheme(scheme.Scheme); err != nil {
		log.Panicf("error while adding [%v] CRD to scheme: %v", c.FullName(), err)
	}
	return c
}

func (d Crd) Register(apiexts apiexts.Interface) error {
	scope := v1beta1.NamespaceScoped
	if d.ClusterScoped {
		scope = v1beta1.ClusterScoped
	}
	toRegister := &v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: d.FullName()},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group:   d.Group,
			Version: d.Version,
			Scope:   scope,
			Names: v1beta1.CustomResourceDefinitionNames{
				Plural:     d.Plural,
				Kind:       d.KindName,
				ShortNames: []string{d.ShortName},
			},
		},
	}
	_, err := apiexts.ApiextensionsV1beta1().CustomResourceDefinitions().Create(toRegister)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to register crd: %v", err)
	}
	return kubeutils.WaitForCrdActive(apiexts, toRegister.Name)
}

func (d Crd) KubeResource(resource resources.InputResource) *v1.Resource {
	data, err := protoutils.MarshalMap(resource)
	if err != nil {
		panic(fmt.Sprintf("internal error: failed to marshal resource to map: %v", err))
	}
	delete(data, "metadata")
	delete(data, "status")
	spec := v1.Spec(data)
	return &v1.Resource{
		TypeMeta: d.TypeMeta(),
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       resource.GetMetadata().Namespace,
			Name:            resource.GetMetadata().Name,
			ResourceVersion: resource.GetMetadata().ResourceVersion,
			Labels:          resource.GetMetadata().Labels,
			Annotations:     resource.GetMetadata().Annotations,
		},
		Status: resource.GetStatus(),
		Spec:   &spec,
	}
}

func (d Crd) FullName() string {
	return d.Plural + "." + d.Group
}

func (d Crd) TypeMeta() metav1.TypeMeta {
	return metav1.TypeMeta{
		Kind:       d.KindName,
		APIVersion: d.Group + "/" + d.Version,
	}
}

// SchemeGroupVersion is group version used to register these objects
func (d Crd) SchemeGroupVersion() schema.GroupVersion {
	return schema.GroupVersion{Group: d.GroupName, Version: d.Version}
}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func (d Crd) Kind(kind string) schema.GroupKind {
	return d.SchemeGroupVersion().WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func (d Crd) Resource(resource string) schema.GroupResource {
	return d.SchemeGroupVersion().WithResource(resource).GroupResource()
}

func (d Crd) SchemeBuilder() runtime.SchemeBuilder {
	return runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypeWithName(d.SchemeGroupVersion().WithKind(d.KindName), &v1.Resource{})
		scheme.AddKnownTypeWithName(d.SchemeGroupVersion().WithKind(d.KindName+"List"), &v1.ResourceList{})

		metav1.AddToGroupVersion(scheme, d.SchemeGroupVersion())
		return nil
	})
}

func (d Crd) AddToScheme(s *runtime.Scheme) error {
	registerLock.Lock()
	defer registerLock.Unlock()
	builder := d.SchemeBuilder()
	return (&builder).AddToScheme(s)
}
