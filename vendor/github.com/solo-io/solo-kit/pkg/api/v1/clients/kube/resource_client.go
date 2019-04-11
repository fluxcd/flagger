package kube

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/solo-io/solo-kit/pkg/utils/stringutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/client/clientset/versioned"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	MCreates        = stats.Int64("kube/creates", "The number of creates", "1")
	CreateCountView = &view.View{
		Name:        "kube/creates-count",
		Measure:     MCreates,
		Description: "The number of create calls",
		Aggregation: view.Count(),
		TagKeys: []tag.Key{
			KeyKind,
		},
	}
	MUpdates        = stats.Int64("kube/updates", "The number of updates", "1")
	UpdateCountView = &view.View{
		Name:        "kube/updates-count",
		Measure:     MUpdates,
		Description: "The number of update calls",
		Aggregation: view.Count(),
		TagKeys: []tag.Key{
			KeyKind,
		},
	}

	MDeletes        = stats.Int64("kube/deletes", "The number of deletes", "1")
	DeleteCountView = &view.View{
		Name:        "kube/deletes-count",
		Measure:     MDeletes,
		Description: "The number of delete calls",
		Aggregation: view.Count(),
		TagKeys: []tag.Key{
			KeyKind,
		},
	}

	KeyOpKind, _ = tag.NewKey("op")

	MInFlight       = stats.Int64("kube/req_in_flight", "The number of requests in flight", "1")
	InFlightSumView = &view.View{
		Name:        "kube/req-in-flight",
		Measure:     MInFlight,
		Description: "The number of requests in flight",
		Aggregation: view.Sum(),
		TagKeys: []tag.Key{
			KeyOpKind,
			KeyKind,
		},
	}

	MEvents         = stats.Int64("kube/events", "The number of events", "1")
	EventsCountView = &view.View{
		Name:        "kube/events-count",
		Measure:     MEvents,
		Description: "The number of events sent from kuberenets to us",
		Aggregation: view.Count(),
	}
)

func init() {
	view.Register(CreateCountView, UpdateCountView, DeleteCountView, InFlightSumView, EventsCountView)
}

// lazy start in list & watch
// register informers in register
type ResourceClient struct {
	crd                crd.Crd
	crdClientset       versioned.Interface
	resourceName       string
	resourceType       resources.InputResource
	sharedCache        SharedCache
	namespaceWhitelist []string // Will contain at least metaV1.NamespaceAll ("")
	resyncPeriod       time.Duration
}

func NewResourceClient(
	crd crd.Crd,
	clientset versioned.Interface,
	sharedCache SharedCache,
	resourceType resources.InputResource,
	namespaceWhitelist []string,
	resyncPeriod time.Duration,
) *ResourceClient {

	typeof := reflect.TypeOf(resourceType)
	resourceName := strings.Replace(typeof.String(), "*", "", -1)
	resourceName = strings.Replace(resourceName, ".", "", -1)

	return &ResourceClient{
		crd:                crd,
		crdClientset:       clientset,
		resourceName:       resourceName,
		resourceType:       resourceType,
		sharedCache:        sharedCache,
		namespaceWhitelist: namespaceWhitelist,
		resyncPeriod:       resyncPeriod,
	}
}

var _ clients.ResourceClient = &ResourceClient{}

func (rc *ResourceClient) Kind() string {
	return resources.Kind(rc.resourceType)
}

func (rc *ResourceClient) NewResource() resources.Resource {
	return resources.Clone(rc.resourceType)
}

// Registers the client with the shared cache. The cache will create a dedicated informer to list and
// watch resources of kind rc.Kind() in the namespaces given in rc.namespaceWhitelist.
func (rc *ResourceClient) Register() error {
	return rc.sharedCache.Register(rc)
}

func (rc *ResourceClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	if err := resources.ValidateName(name); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	opts = opts.WithDefaults()

	if err := rc.validateNamespace(namespace); err != nil {
		return nil, err
	}

	ctx := opts.Ctx

	if ctxWithTags, err := tag.New(ctx, tag.Insert(KeyKind, rc.resourceName), tag.Insert(KeyOpKind, "read")); err == nil {
		ctx = ctxWithTags
	}

	stats.Record(ctx, MInFlight.M(1))
	resourceCrd, err := rc.crdClientset.ResourcesV1().Resources(namespace).Get(name, metav1.GetOptions{})
	stats.Record(ctx, MInFlight.M(-1))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewNotExistErr(namespace, name, err)
		}
		return nil, errors.Wrapf(err, "reading resource from kubernetes")
	}
	resource, err := rc.convertCrdToResource(resourceCrd)
	if err != nil {
		return nil, errors.Wrapf(err, "converting output crd")
	}
	return resource, nil
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()

	if err := rc.validateNamespace(meta.Namespace); err != nil {
		return nil, err
	}

	// mutate and return clone
	clone := resources.Clone(resource).(resources.InputResource)
	clone.SetMetadata(meta)
	resourceCrd := rc.crd.KubeResource(clone)

	ctx := opts.Ctx
	if ctxWithTags, err := tag.New(ctx, tag.Insert(KeyKind, rc.resourceName), tag.Insert(KeyOpKind, "write")); err == nil {
		ctx = ctxWithTags
	}

	if rc.exist(ctx, meta.Namespace, meta.Name) {
		if !opts.OverwriteExisting {
			return nil, errors.NewExistErr(meta)
		}
		stats.Record(ctx, MUpdates.M(1), MInFlight.M(1))
		defer stats.Record(ctx, MInFlight.M(-1))
		if _, updateErr := rc.crdClientset.ResourcesV1().Resources(meta.Namespace).Update(resourceCrd); updateErr != nil {
			original, err := rc.crdClientset.ResourcesV1().Resources(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			if err == nil {
				return nil, errors.Wrapf(updateErr, "updating kube resource %v:%v (want %v)", resourceCrd.Name, resourceCrd.ResourceVersion, original.ResourceVersion)
			}
			return nil, errors.Wrapf(updateErr, "updating kube resource %v", resourceCrd.Name)
		}
	} else {
		stats.Record(ctx, MCreates.M(1), MInFlight.M(1))
		defer stats.Record(ctx, MInFlight.M(-1))
		if _, err := rc.crdClientset.ResourcesV1().Resources(meta.Namespace).Create(resourceCrd); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil, errors.NewExistErr(meta)
			}
			return nil, errors.Wrapf(err, "creating kube resource %v", resourceCrd.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(meta.Namespace, meta.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {

	if err := rc.validateNamespace(namespace); err != nil {
		return err
	}

	opts = opts.WithDefaults()

	ctx := opts.Ctx

	if ctxWithTags, err := tag.New(ctx, tag.Insert(KeyKind, rc.resourceName), tag.Insert(KeyOpKind, "delete")); err == nil {
		ctx = ctxWithTags
	}
	stats.Record(ctx, MDeletes.M(1))

	if !rc.exist(ctx, namespace, name) {
		if !opts.IgnoreNotExist {
			return errors.NewNotExistErr(namespace, name)
		}
		return nil
	}

	stats.Record(ctx, MInFlight.M(1))
	defer stats.Record(ctx, MInFlight.M(-1))
	if err := rc.crdClientset.ResourcesV1().Resources(namespace).Delete(name, nil); err != nil {
		return errors.Wrapf(err, "deleting resource %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	if err := rc.validateNamespace(namespace); err != nil {
		return nil, err
	}

	// Will have no effect if the factory is already running
	rc.sharedCache.Start()

	lister, err := rc.sharedCache.GetLister(namespace, rc.crd.Type)
	if err != nil {
		return nil, err
	}
	allResources, err := lister.List(labels.SelectorFromSet(opts.Selector))
	if err != nil {
		return nil, errors.Wrapf(err, "listing resources in %v", namespace)
	}
	var listedResources []*v1.Resource
	if namespace != "" {
		for _, r := range allResources {
			if r.ObjectMeta.Namespace == namespace {
				listedResources = append(listedResources, r)
			}
		}
	} else {
		listedResources = allResources
	}

	var resourceList resources.ResourceList
	for _, resourceCrd := range listedResources {
		resource, err := rc.convertCrdToResource(resourceCrd)
		if err != nil {
			return nil, errors.Wrapf(err, "converting output crd")
		}
		resourceList = append(resourceList, resource)
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().Name < resourceList[j].GetMetadata().Name
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {

	if err := rc.validateNamespace(namespace); err != nil {
		return nil, nil, err
	}

	rc.sharedCache.Start()

	opts = opts.WithDefaults()
	resourcesChan := make(chan resources.ResourceList)
	errs := make(chan error)
	ctx := opts.Ctx

	updateResourceList := func() {
		list, err := rc.List(namespace, clients.ListOpts{
			Ctx:      ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		resourcesChan <- list
	}
	// watch should open up with an initial read
	cacheUpdated := rc.sharedCache.AddWatch(10)

	go func(watchedNamespace string) {
		defer rc.sharedCache.RemoveWatch(cacheUpdated)
		defer close(resourcesChan)
		defer close(errs)

		// Perform an initial list operation
		updateResourceList()

		for {
			select {
			case resource := <-cacheUpdated:

				// Only notify watchers if the updated resource is in the watched
				// namespace and its kind matches the one of the resource clientz
				if matchesTargetNamespace(watchedNamespace, resource.ObjectMeta.Namespace) && rc.matchesClientKind(resource) {
					updateResourceList()
				}
			case <-ctx.Done():
				return
			}
		}

	}(namespace)

	return resourcesChan, errs, nil
}

func matchesTargetNamespace(targetNs, resourceNs string) bool {
	// "" == all namespaces are valid
	if targetNs == "" {
		return true
	}
	return targetNs == resourceNs
}

// Checks whether the type of the given resource matches the one of the client's underlying CRD:
// 1. the kind name must match that of CRD
// 2. the version must match the CRD GroupVersion (in the form <GROUP_NAME>/<VERSION>)
func (rc *ResourceClient) matchesClientKind(resource v1.Resource) bool {
	return resource.Kind == rc.crd.KindName && resource.APIVersion == rc.crd.SchemeGroupVersion().String()
}

func (rc *ResourceClient) exist(ctx context.Context, namespace, name string) bool {

	if ctxWithTags, err := tag.New(ctx, tag.Insert(KeyKind, rc.resourceName), tag.Upsert(KeyOpKind, "get")); err == nil {
		ctx = ctxWithTags
	}

	stats.Record(ctx, MInFlight.M(1))
	defer stats.Record(ctx, MInFlight.M(-1))

	_, err := rc.crdClientset.ResourcesV1().Resources(namespace).Get(name, metav1.GetOptions{}) // TODO(yuval-k): check error for real
	return err == nil

}

func (rc *ResourceClient) convertCrdToResource(resourceCrd *v1.Resource) (resources.Resource, error) {
	resource := rc.NewResource()
	if resourceCrd.Spec != nil {
		if err := protoutils.UnmarshalMap(*resourceCrd.Spec, resource); err != nil {
			return nil, errors.Wrapf(err, "reading crd spec into %v", rc.resourceName)
		}
	}
	resource.SetMetadata(kubeutils.FromKubeMeta(resourceCrd.ObjectMeta))
	if withStatus, ok := resource.(resources.InputResource); ok {
		resources.UpdateStatus(withStatus, func(status *core.Status) {
			*status = resourceCrd.Status
		})
	}
	return resource, nil
}

// Check whether the given namespace is in the whitelist or we allow all namespaces
func (rc *ResourceClient) validateNamespace(namespace string) error {
	if !stringutils.ContainsAny([]string{namespace, metav1.NamespaceAll}, rc.namespaceWhitelist) {
		return errors.Errorf("this client was not configured to access resources in the [%v] namespace. "+
			"Allowed namespaces are %v", namespace, rc.namespaceWhitelist)
	}
	return nil
}
