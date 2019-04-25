package resources

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"
)

const delim = " "

type Resource interface {
	GetMetadata() core.Metadata
	SetMetadata(meta core.Metadata)
	Equal(that interface{}) bool
}

type ProtoResource interface {
	Resource
	proto.Message
}

func ProtoCast(res Resource) (ProtoResource, error) {
	if res == nil {
		return nil, nil
	}
	protoResource, ok := res.(ProtoResource)
	if !ok {
		return nil, errors.Errorf("internal error: unexpected type %T not convertible to resources.Proto", res)
	}
	return protoResource, nil
}

func Key(resource Resource) string {
	return fmt.Sprintf("%v%v%v%v%v", Kind(resource), delim, resource.GetMetadata().Namespace, delim,
		resource.GetMetadata().Name)
}

func SplitKey(key string) (string, string, string, error) {
	parts := strings.Split(key, delim)
	if len(parts) != 3 {
		return "", "", "", errors.Errorf("%v was not a valid key", key)
	}
	kind := parts[0]
	namespace := parts[1]
	name := parts[2]
	return kind, namespace, name, nil
}

type InputResource interface {
	Resource
	GetStatus() core.Status
	SetStatus(status core.Status)
}

type ResourceList []Resource
type ResourcesById map[string]Resource
type ResourcesByKind map[string]ResourceList

func (m ResourcesById) List() ResourceList {
	var all ResourceList
	for _, res := range m {
		all = append(all, res)
	}
	// sort by type
	sort.SliceStable(all, func(i, j int) bool {
		return Key(all[i]) < Key(all[j])
	})
	return all
}

func (m ResourcesByKind) Add(resources ...Resource) {
	for _, resource := range resources {
		m[Kind(resource)] = append(m[Kind(resource)], resource)
	}
}
func (m ResourcesByKind) Get(resource Resource) []Resource {
	return m[Kind(resource)]
}
func (m ResourcesByKind) List() ResourceList {
	var all ResourceList
	for _, list := range m {
		all = append(all, list...)
	}
	// sort by type
	sort.SliceStable(all, func(i, j int) bool {
		return Key(all[i]) < Key(all[j])
	})
	return all
}
func (list ResourceList) Contains(list2 ResourceList) bool {
	for _, res2 := range list2 {
		var found bool
		for _, res := range list {
			if res.GetMetadata().Name == res2.GetMetadata().Name && res.GetMetadata().Namespace == res2.GetMetadata().Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (list ResourceList) Copy() ResourceList {
	var cpy ResourceList
	for _, res := range list {
		cpy = append(cpy, Clone(res))
	}
	return cpy
}
func (list ResourceList) Equal(list2 ResourceList) bool {
	if len(list) != len(list2) {
		return false
	}
	for i := range list {
		if !list[i].Equal(list2[i]) {
			return false
		}
	}
	return true
}
func (list ResourceList) FilterByNames(names []string) ResourceList {
	var filtered ResourceList
	for _, resource := range list {
		for _, name := range names {
			if name == resource.GetMetadata().Name {
				filtered = append(filtered, resource)
				break
			}
		}
	}
	return filtered
}
func (list ResourceList) FilterByNamespaces(namespaces []string) ResourceList {
	var filtered ResourceList
	for _, resource := range list {
		for _, namespace := range namespaces {
			if namespace == resource.GetMetadata().Namespace {
				filtered = append(filtered, resource)
				break
			}
		}
	}
	return filtered
}
func (list ResourceList) FilterByKind(kind string) ResourceList {
	var resourcesOfKind ResourceList
	for _, res := range list {
		if Kind(res) == kind {
			resourcesOfKind = append(resourcesOfKind, res)
		}
	}
	return resourcesOfKind
}
func (list ResourceList) FilterByList(list2 ResourceList) ResourceList {
	return list.FilterByNamespaces(list2.Namespaces()).FilterByNames(list.Names())
}
func (list ResourceList) Names() []string {
	var names []string
	for _, resource := range list {
		names = append(names, resource.GetMetadata().Name)
	}
	return names
}
func (list ResourceList) Find(namespace, name string) (Resource, error) {
	for _, resource := range list {
		if resource.GetMetadata().Name == name {
			if namespace == "" || resource.GetMetadata().Namespace == namespace {
				return resource, nil
			}
		}
	}
	return nil, errors.Errorf("list did not find resource %v.%v", namespace, name)
}
func (list ResourceList) Namespaces() []string {
	var namespaces []string
	for _, resource := range list {
		namespaces = append(namespaces, resource.GetMetadata().Namespace)
	}
	return namespaces
}
func (list ResourceList) AsInputResourceList() InputResourceList {
	var inputs InputResourceList
	for _, res := range list {
		inputRes, ok := res.(InputResource)
		if !ok {
			continue
		}
		inputs = append(inputs, inputRes)
	}
	return inputs
}

type InputResourceList []InputResource
type InputResourcesByKind map[string]InputResourceList

func (m InputResourcesByKind) Add(resource InputResource) {
	m[Kind(resource)] = append(m[Kind(resource)], resource)
}
func (m InputResourcesByKind) Get(resource InputResource) InputResourceList {
	return m[Kind(resource)]
}
func (m InputResourcesByKind) List() InputResourceList {
	var all InputResourceList
	for _, list := range m {
		all = append(all, list...)
	}
	// sort by type
	sort.SliceStable(all, func(i, j int) bool {
		return Key(all[i]) < Key(all[j])
	})
	return all
}
func (list InputResourceList) Contains(list2 InputResourceList) bool {
	for _, res2 := range list2 {
		var found bool
		for _, res := range list {
			if res.GetMetadata().Name == res2.GetMetadata().Name && res.GetMetadata().Namespace == res2.GetMetadata().Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (list InputResourceList) Copy() InputResourceList {
	var cpy InputResourceList
	for _, res := range list {
		cpy = append(cpy, Clone(res).(InputResource))
	}
	return cpy
}
func (list InputResourceList) Equal(list2 InputResourceList) bool {
	if len(list) != len(list2) {
		return false
	}
	for i := range list {
		if !list[i].Equal(list2[i]) {
			return false
		}
	}
	return true
}
func (list InputResourceList) FilterByNames(names []string) InputResourceList {
	var filtered InputResourceList
	for _, resource := range list {
		for _, name := range names {
			if name == resource.GetMetadata().Name {
				filtered = append(filtered, resource)
				break
			}
		}
	}
	return filtered
}
func (list InputResourceList) FilterByNamespaces(namespaces []string) InputResourceList {
	var filtered InputResourceList
	for _, resource := range list {
		for _, namespace := range namespaces {
			if namespace == resource.GetMetadata().Namespace {
				filtered = append(filtered, resource)
				break
			}
		}
	}
	return filtered
}
func (list InputResourceList) FilterByKind(kind string) InputResourceList {
	var resourcesOfKind InputResourceList
	for _, res := range list {
		if Kind(res) == kind {
			resourcesOfKind = append(resourcesOfKind, res)
		}
	}
	return resourcesOfKind
}
func (list InputResourceList) FilterByList(list2 InputResourceList) InputResourceList {
	return list.FilterByNamespaces(list2.Namespaces()).FilterByNames(list.Names())
}
func (list InputResourceList) Find(namespace, name string) (InputResource, error) {
	for _, resource := range list {
		if resource.GetMetadata().Name == name {
			if namespace == "" || resource.GetMetadata().Namespace == namespace {
				return resource, nil
			}
		}
	}
	return nil, errors.Errorf("list did not find resource %v.%v", namespace, name)
}
func (list InputResourceList) Names() []string {
	var names []string
	for _, resource := range list {
		names = append(names, resource.GetMetadata().Name)
	}
	return names
}
func (list InputResourceList) Namespaces() []string {
	var namespaces []string
	for _, resource := range list {
		namespaces = append(namespaces, resource.GetMetadata().Namespace)
	}
	return namespaces
}
func (list InputResourceList) AsResourceList() ResourceList {
	var resources ResourceList
	for _, res := range list {
		resources = append(resources, res)
	}
	return resources
}

type HashableResource interface {
	Resource
	Hash() uint64
}

type CloneableResource interface {
	Resource
	Clone() Resource
}

func Clone(resource Resource) Resource {
	if cloneable, ok := resource.(CloneableResource); ok {
		return cloneable.Clone()
	}
	if protoMessage, ok := resource.(ProtoResource); ok {
		return proto.Clone(protoMessage).(Resource)
	}
	panic(fmt.Errorf("resource %T is not cloneable and not a proto", resource))
}

func Kind(resource Resource) string {
	return reflect.TypeOf(resource).String()
}

func UpdateMetadata(resource Resource, updateFunc func(meta *core.Metadata)) {
	meta := resource.GetMetadata()
	updateFunc(&meta)
	resource.SetMetadata(meta)
}

func UpdateStatus(resource InputResource, updateFunc func(status *core.Status)) {
	status := resource.GetStatus()
	updateFunc(&status)
	resource.SetStatus(status)
}

func Validate(resource Resource) error {
	return ValidateName(resource.GetMetadata().Name)
}

func ValidateName(name string) error {
	errs := validation.IsDNS1123Subdomain(name)
	if len(name) < 1 {
		errs = append(errs, "name cannot be empty. Given: "+name)
	}
	if len(name) > 253 {
		errs = append(errs, "name has a max length of 253 characters. Given: "+name)
	}
	if len(errs) > 0 {
		return errors.Errors(errs)
	}
	return nil
}
