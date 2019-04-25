package kubeutils

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func FromKubeMeta(meta metav1.ObjectMeta) core.Metadata {
	return core.Metadata{
		Name:            meta.Name,
		Namespace:       meta.Namespace,
		ResourceVersion: meta.ResourceVersion,
		Labels:          copyMap(meta.Labels),
		Annotations:     copyMap(meta.Annotations),
	}
}

func ToKubeMeta(meta core.Metadata) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            meta.Name,
		Namespace:       clients.DefaultNamespaceIfEmpty(meta.Namespace),
		ResourceVersion: meta.ResourceVersion,
		Labels:          copyMap(meta.Labels),
		Annotations:     copyMap(meta.Annotations),
	}
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	res := map[string]string{}
	for k, v := range m {
		res[k] = v
	}
	return res
}
