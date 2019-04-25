package configmap

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	v1 "k8s.io/api/core/v1"
)

type ConfigMapConverter interface {
	FromKubeConfigMap(ctx context.Context, rc *ResourceClient, configMap *v1.ConfigMap) (resources.Resource, error)
	ToKubeConfigMap(ctx context.Context, rc *ResourceClient, resource resources.Resource) (*v1.ConfigMap, error)
}

type structConverter struct{}

func (cc *structConverter) FromKubeConfigMap(ctx context.Context, rc *ResourceClient, configMap *v1.ConfigMap) (resources.Resource, error) {
	resource := rc.NewResource()
	// not our configMap
	// should be an error on a Read, ignored on a list
	if len(configMap.ObjectMeta.Annotations) == 0 || configMap.ObjectMeta.Annotations[annotationKey] != rc.Kind() {
		return nil, nil
	}
	// convert mapstruct to our object
	resourceMap, err := protoutils.MapStringStringToMapStringInterface(configMap.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing configmap data as map[string]interface{}")
	}

	if err := protoutils.UnmarshalMap(resourceMap, resource); err != nil {
		return nil, errors.Wrapf(err, "reading configmap data into %v", rc.Kind())
	}
	resource.SetMetadata(kubeutils.FromKubeMeta(configMap.ObjectMeta))

	return resource, nil
}

func (cc *structConverter) ToKubeConfigMap(ctx context.Context, rc *ResourceClient, resource resources.Resource) (*v1.ConfigMap, error) {
	resourceMap, err := protoutils.MarshalMap(resource)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling resource as map")
	}
	resourceData, err := protoutils.MapStringInterfaceToMapStringString(resourceMap)
	if err != nil {
		return nil, errors.Wrapf(err, "internal err: converting resource map to map[string]string")
	}
	// metadata moves over to kube style
	delete(resourceData, "metadata")
	meta := kubeutils.ToKubeMeta(resource.GetMetadata())
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	meta.Annotations[annotationKey] = rc.Kind()
	return &v1.ConfigMap{
		ObjectMeta: meta,
		Data:       resourceData,
	}, nil
}

type plainConverter struct{}

func (cc *plainConverter) FromKubeConfigMap(ctx context.Context, rc *ResourceClient, configMap *v1.ConfigMap) (resources.Resource, error) {
	resource := rc.NewResource()
	// not our configMap
	// should be an error on a Read, ignored on a list
	if len(configMap.ObjectMeta.Annotations) == 0 || configMap.ObjectMeta.Annotations[annotationKey] != rc.Kind() {
		return nil, nil
	}
	// only works for string fields
	resourceMap := make(map[string]interface{})
	for k, v := range configMap.Data {
		resourceMap[k] = v
	}

	if err := protoutils.UnmarshalMap(resourceMap, resource); err != nil {
		return nil, errors.Wrapf(err, "reading configmap data into %v", rc.Kind())
	}
	resource.SetMetadata(kubeutils.FromKubeMeta(configMap.ObjectMeta))

	return resource, nil
}

func (cc *plainConverter) ToKubeConfigMap(ctx context.Context, rc *ResourceClient, resource resources.Resource) (*v1.ConfigMap, error) {
	resourceMap, err := protoutils.MarshalMapEmitZeroValues(resource)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling resource as map")
	}
	configMapData := make(map[string]string)
	for k, v := range resourceMap {
		// metadata comes from ToKubeMeta
		// status not supported
		if k == "metadata" {
			continue
		}
		switch val := v.(type) {
		case string:
			configMapData[k] = val
		default:
			// TODO: handle other field types; for now the caller
			// must know this resource client only supports map[string]string style objects
			contextutils.LoggerFrom(ctx).Warnf("invalid resource type (%v) used for plain configmap. unable to "+
				"convert to kube configmap. only resources with fields of type string are supported for the plain configmap client.", resources.Kind(resource))
		}
	}
	meta := kubeutils.ToKubeMeta(resource.GetMetadata())
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	meta.Annotations[annotationKey] = rc.Kind()
	return &v1.ConfigMap{
		ObjectMeta: meta,
		Data:       configMapData,
	}, nil
}
