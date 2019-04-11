package configmap

import (
	"reflect"
	"sort"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const annotationKey = "resource_kind"

type ResourceClient struct {
	apiexts      apiexts.Interface
	kube         kubernetes.Interface
	ownerLabel   string
	resourceName string
	resourceType resources.Resource
	kubeCache    cache.KubeCoreCache
	// custom logic to convert the configmap to a resource
	converter ConfigMapConverter
}

func NewResourceClient(kube kubernetes.Interface, resourceType resources.Resource, kubeCache cache.KubeCoreCache, plainConfigMaps bool) (*ResourceClient, error) {
	var configmapConverter ConfigMapConverter = &structConverter{}
	if plainConfigMaps {
		configmapConverter = &plainConverter{}
	}
	return NewResourceClientWithConverter(kube, resourceType, kubeCache, configmapConverter)
}

func NewResourceClientWithConverter(kube kubernetes.Interface, resourceType resources.Resource, kubeCache cache.KubeCoreCache, secretConverter ConfigMapConverter) (*ResourceClient, error) {
	return &ResourceClient{
		kube:         kube,
		resourceName: reflect.TypeOf(resourceType).String(),
		resourceType: resourceType,
		kubeCache:    kubeCache,
		converter:    secretConverter,
	}, nil
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

	configMap, err := rc.kube.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewNotExistErr(namespace, name, err)
		}
		return nil, errors.Wrapf(err, "reading configMap from kubernetes")
	}
	resource, err := rc.converter.FromKubeConfigMap(opts.Ctx, rc, configMap)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, errors.Errorf("configMap %v is not kind %v", name, rc.Kind())
	}
	return resource, nil
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()

	// mutate and return clone
	clone := resources.Clone(resource)
	clone.SetMetadata(meta)
	configMap, err := rc.converter.ToKubeConfigMap(opts.Ctx, rc, resource)
	if err != nil {
		return nil, err
	}

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
		if _, err := rc.kube.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap); err != nil {
			return nil, errors.Wrapf(err, "updating kube configMap %v", configMap.Name)
		}
	} else {
		if _, err := rc.kube.CoreV1().ConfigMaps(configMap.Namespace).Create(configMap); err != nil {
			return nil, errors.Wrapf(err, "creating kube configMap %v", configMap.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(configMap.Namespace, configMap.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts = opts.WithDefaults()
	if !rc.exist(namespace, name) {
		if !opts.IgnoreNotExist {
			return errors.NewNotExistErr(namespace, name)
		}
		return nil
	}

	if err := rc.kube.CoreV1().ConfigMaps(namespace).Delete(name, nil); err != nil {
		return errors.Wrapf(err, "deleting configMap %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()

	configMapList, err := rc.kubeCache.ConfigMapLister().ConfigMaps(namespace).List(labels.SelectorFromSet(opts.Selector))
	if err != nil {
		return nil, errors.Wrapf(err, "listing configMaps in %v", namespace)
	}
	var resourceList resources.ResourceList
	for _, configMap := range configMapList {
		resource, err := rc.converter.FromKubeConfigMap(opts.Ctx, rc, configMap)
		if err != nil {
			return nil, err
		}
		if resource == nil {
			continue
		}
		resourceList = append(resourceList, resource)
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().Name < resourceList[j].GetMetadata().Name
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	opts = opts.WithDefaults()
	watch := rc.kubeCache.Subscribe()

	resourcesChan := make(chan resources.ResourceList)
	errs := make(chan error)

	// prevent flooding the channel with duplicates
	var previous *resources.ResourceList
	updateResourceList := func() {
		list, err := rc.List(namespace, clients.ListOpts{
			Ctx:      opts.Ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		if previous != nil {
			if list.Equal(*previous) {
				return
			}
		}
		previous = &list
		resourcesChan <- list
	}

	go func() {
		defer rc.kubeCache.Unsubscribe(watch)
		defer close(resourcesChan)
		defer close(errs)

		// watch should open up with an initial read
		updateResourceList()
		for {
			select {
			case _, ok := <-watch:
				if !ok {
					return
				}
				updateResourceList()
			case <-opts.Ctx.Done():
				return
			}
		}
	}()

	return resourcesChan, errs, nil
}

func (rc *ResourceClient) exist(namespace, name string) bool {
	_, err := rc.kube.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	return err == nil
}
