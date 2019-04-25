package consul

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"k8s.io/apimachinery/pkg/labels"
)

type ResourceClient struct {
	consul       *api.Client
	root         string
	resourceType resources.Resource
}

func NewResourceClient(client *api.Client, rootKey string, resourceType resources.Resource) *ResourceClient {
	return &ResourceClient{
		consul:       client,
		root:         rootKey,
		resourceType: resourceType,
	}
}

var _ clients.ResourceClient = &ResourceClient{}

func (rc *ResourceClient) Kind() string {
	return resources.Kind(rc.resourceType)
}

func (rc *ResourceClient) NewResource() resources.Resource {
	return resources.Clone(rc.resourceType)
}

func (rc *ResourceClient) Register() error {
	return nil
}

func (rc *ResourceClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	if err := resources.ValidateName(name); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	opts = opts.WithDefaults()
	key := rc.resourceKey(namespace, name)

	kvPair, _, err := rc.consul.KV().Get(key, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "performing consul KV get")
	}
	if kvPair == nil {
		return nil, errors.NewNotExistErr(namespace, name)
	}
	resource := rc.NewResource()
	if err := protoutils.UnmarshalBytes(kvPair.Value, resource); err != nil {
		return nil, errors.Wrapf(err, "reading KV into %v", rc.Kind())
	}
	resources.UpdateMetadata(resource, func(meta *core.Metadata) {
		meta.ResourceVersion = fmt.Sprintf("%v", kvPair.ModifyIndex)
	})
	return resource, nil
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.Namespace)
	key := rc.resourceKey(meta.Namespace, meta.Name)

	original, err := rc.Read(meta.Namespace, meta.Name, clients.ReadOpts{
		Ctx: opts.Ctx,
	})
	if original != nil && err == nil {
		if !opts.OverwriteExisting {
			return nil, errors.NewExistErr(meta)
		}
		if meta.ResourceVersion != original.GetMetadata().ResourceVersion {
			return nil, errors.NewResourceVersionErr(meta.Namespace, meta.Name, meta.ResourceVersion, original.GetMetadata().ResourceVersion)
		}
	}

	// mutate and return clone
	clone := resources.Clone(resource)
	clone.SetMetadata(meta)

	data, err := protoutils.MarshalBytes(clone)
	if err != nil {
		panic(errors.Wrapf(err, "internal err: failed to marshal resource"))
	}
	var modifyIndex uint64
	if meta.GetResourceVersion() != "" {
		if i, err := strconv.Atoi(meta.GetResourceVersion()); err == nil {
			modifyIndex = uint64(i)
		}
	}
	kvPair := &api.KVPair{
		Key:         key,
		Value:       data,
		ModifyIndex: modifyIndex,
	}
	if _, err := rc.consul.KV().Put(kvPair, nil); err != nil {
		return nil, errors.Wrapf(err, "writing to KV")
	}
	// return a read object to update the modify index
	return rc.Read(meta.Namespace, meta.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts = opts.WithDefaults()
	namespace = clients.DefaultNamespaceIfEmpty(namespace)
	key := rc.resourceKey(namespace, name)
	if !opts.IgnoreNotExist {
		if _, err := rc.Read(namespace, name, clients.ReadOpts{Ctx: opts.Ctx}); err != nil {
			return errors.NewNotExistErr(namespace, name, err)
		}
	}
	_, err := rc.consul.KV().Delete(key, nil)
	if err != nil {
		return errors.Wrapf(err, "deleting resource %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()
	namespace = clients.DefaultNamespaceIfEmpty(namespace)

	namespacePrefix := filepath.Join(rc.root, namespace)
	kvPairs, _, err := rc.consul.KV().List(namespacePrefix, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "reading namespace root")
	}

	var resourceList resources.ResourceList
	for _, kvPair := range kvPairs {
		resource := rc.NewResource()
		if err := protoutils.UnmarshalBytes(kvPair.Value, resource); err != nil {
			return nil, errors.Wrapf(err, "reading KV into %v", rc.Kind())
		}
		resources.UpdateMetadata(resource, func(meta *core.Metadata) {
			meta.ResourceVersion = fmt.Sprintf("%v", kvPair.ModifyIndex)
		})
		if labels.SelectorFromSet(opts.Selector).Matches(labels.Set(resource.GetMetadata().Labels)) {
			resourceList = append(resourceList, resource)
		}
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().Name < resourceList[j].GetMetadata().Name
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	opts = opts.WithDefaults()
	var lastIndex uint64
	namespace = clients.DefaultNamespaceIfEmpty(namespace)
	namespacePrefix := filepath.Join(rc.root, namespace)
	resourcesChan := make(chan resources.ResourceList)
	errs := make(chan error)
	go func() {
		// watch should open up with an initial read
		list, err := rc.List(namespace, clients.ListOpts{
			Ctx:      opts.Ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		resourcesChan <- list
	}()
	updatedResourceList := func() (resources.ResourceList, error) {
		kvPairs, meta, err := rc.consul.KV().List(namespacePrefix,
			&api.QueryOptions{
				RequireConsistent: true,
				WaitIndex:         lastIndex,
				WaitTime:          opts.RefreshRate,
			})
		if err != nil {
			return nil, errors.Wrapf(err, "getting kv-pairs list")
		}
		// no change since last poll
		if lastIndex == meta.LastIndex {
			return nil, nil
		}
		var resourceList resources.ResourceList
		for _, kvPair := range kvPairs {
			resource := rc.NewResource()
			if err := protoutils.UnmarshalBytes(kvPair.Value, resource); err != nil {
				return nil, errors.Wrapf(err, "reading KV into %v", rc.Kind())
			}
			resources.UpdateMetadata(resource, func(meta *core.Metadata) {
				meta.ResourceVersion = fmt.Sprintf("%v", kvPair.ModifyIndex)
			})
			if labels.SelectorFromSet(opts.Selector).Matches(labels.Set(resource.GetMetadata().Labels)) {
				resourceList = append(resourceList, resource)
			}
		}

		sort.SliceStable(resourceList, func(i, j int) bool {
			return resourceList[i].GetMetadata().Name < resourceList[j].GetMetadata().Name
		})

		// update index
		lastIndex = meta.LastIndex
		return resourceList, nil
	}

	go func() {
		for {
			select {
			default:
				list, err := updatedResourceList()
				if err != nil {
					errs <- err
				}
				if list != nil {
					resourcesChan <- list
				}
			case <-opts.Ctx.Done():
				close(resourcesChan)
				close(errs)
				return
			}
		}
	}()

	return resourcesChan, errs, nil
}

func (rc *ResourceClient) resourceKey(namespace, name string) string {
	return filepath.Join(rc.root, namespace, name)
}
