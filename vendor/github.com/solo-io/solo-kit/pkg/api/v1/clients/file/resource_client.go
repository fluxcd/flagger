package file

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/radovskyb/watcher"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/fileutils"
	"k8s.io/apimachinery/pkg/labels"
)

func ignoreFile(filename string) bool {
	if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
		return false
	}
	return true
}

type ResourceClient struct {
	dir          string
	resourceType resources.Resource
}

func NewResourceClient(dir string, resourceType resources.Resource) *ResourceClient {
	return &ResourceClient{
		dir:          dir,
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
	namespace = clients.DefaultNamespaceIfEmpty(namespace)
	opts = opts.WithDefaults()
	path := rc.filename(namespace, name)
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return nil, errors.NewNotExistErr(namespace, name, err)
	}
	resource := rc.NewResource()
	if err := fileutils.ReadFileInto(path, resource); err != nil {
		return nil, errors.Wrapf(err, "reading file into %v", rc.Kind())
	}
	return resource, nil
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.Namespace)

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
	// initialize or increment resource version
	meta.ResourceVersion = newOrIncrementResourceVer(meta.ResourceVersion)
	clone.SetMetadata(meta)

	path := rc.filename(meta.Namespace, meta.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && !os.IsExist(err) {
		return nil, errors.Wrapf(err, "creating directory")
	}
	if err := fileutils.WriteToFile(path, clone); err != nil {
		return nil, errors.Wrapf(err, "writing file")
	}
	return clone, nil
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts = opts.WithDefaults()
	namespace = clients.DefaultNamespaceIfEmpty(namespace)
	path := rc.filename(namespace, name)
	err := os.Remove(path)
	switch {
	case err == nil:
		return nil
	case os.IsNotExist(err) && opts.IgnoreNotExist:
		return nil
	case os.IsNotExist(err) && !opts.IgnoreNotExist:
		return errors.NewNotExistErr(namespace, name, err)
	}
	return errors.Wrapf(err, "deleting resource %v", name)
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()
	namespace = clients.DefaultNamespaceIfEmpty(namespace)

	namespaceDir := filepath.Join(rc.dir, namespace)
	files, err := ioutil.ReadDir(namespaceDir)
	if err != nil {
		return nil, errors.Wrapf(err, "reading namespace dir")
	}

	var resourceList resources.ResourceList
	for _, file := range files {
		if ignoreFile(file.Name()) {
			continue
		}
		resource := rc.NewResource()
		path := filepath.Join(namespaceDir, file.Name())
		if err := fileutils.ReadFileInto(path, resource); err != nil {
			return nil, errors.Wrapf(err, "reading file into %v", rc.Kind())
		}
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
	namespace = clients.DefaultNamespaceIfEmpty(namespace)

	dir := filepath.Join(rc.dir, namespace)
	events, errs, err := rc.events(opts.Ctx, dir, opts.RefreshRate)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "starting watch on namespace dir")
	}
	resourcesChan := make(chan resources.ResourceList)
	updateResourceList := func() {
		list, err := rc.List(namespace, clients.ListOpts{
			Ctx:      opts.Ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		resourcesChan <- list
	}
	go func() {
		updateResourceList()
		for {
			select {
			case <-time.After(opts.RefreshRate):
				updateResourceList()
			case <-events:
				updateResourceList()
			case <-opts.Ctx.Done():
				close(resourcesChan)
				close(errs)
				return
			}
		}
	}()

	return resourcesChan, errs, nil
}

func (rc *ResourceClient) filename(namespace, name string) string {
	return filepath.Join(rc.dir, namespace, name) + ".yaml"
}

func (rc *ResourceClient) events(ctx context.Context, dir string, refreshRate time.Duration) (<-chan struct{}, chan error, error) {
	events := make(chan struct{})
	errs := make(chan error)
	w := watcher.New()
	w.SetMaxEvents(0)
	w.FilterOps(watcher.Create, watcher.Write, watcher.Remove, watcher.Rename, watcher.Move)
	if err := w.AddRecursive(dir); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to watch directory %v", dir)
	}
	go func() {
		if err := w.Start(refreshRate); err != nil {
			errs <- err
		}
	}()
	go func() {
		for {
			select {
			case event := <-w.Event:
				if event.IsDir() {
					continue
				}
				events <- struct{}{}
			case err := <-w.Error:
				errs <- errors.Wrapf(err, "file watcher error")
			case <-ctx.Done():
				w.Close()
				return
			}
		}
	}()
	return events, errs, nil
}

// util methods
func newOrIncrementResourceVer(resourceVersion string) string {
	curr, err := strconv.Atoi(resourceVersion)
	if err != nil {
		curr = 1
	}
	return fmt.Sprintf("%v", curr+1)
}
