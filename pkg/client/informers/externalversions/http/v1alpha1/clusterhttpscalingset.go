/*
Copyright 2020 The Flux authors

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	httpv1alpha1 "github.com/fluxcd/flagger/pkg/apis/http/v1alpha1"
	versioned "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	internalinterfaces "github.com/fluxcd/flagger/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/fluxcd/flagger/pkg/client/listers/http/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// ClusterHTTPScalingSetInformer provides access to a shared informer and lister for
// ClusterHTTPScalingSets.
type ClusterHTTPScalingSetInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ClusterHTTPScalingSetLister
}

type clusterHTTPScalingSetInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewClusterHTTPScalingSetInformer constructs a new informer for ClusterHTTPScalingSet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewClusterHTTPScalingSetInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredClusterHTTPScalingSetInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredClusterHTTPScalingSetInformer constructs a new informer for ClusterHTTPScalingSet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredClusterHTTPScalingSetInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.HttpV1alpha1().ClusterHTTPScalingSets(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.HttpV1alpha1().ClusterHTTPScalingSets(namespace).Watch(context.TODO(), options)
			},
		},
		&httpv1alpha1.ClusterHTTPScalingSet{},
		resyncPeriod,
		indexers,
	)
}

func (f *clusterHTTPScalingSetInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredClusterHTTPScalingSetInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *clusterHTTPScalingSetInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&httpv1alpha1.ClusterHTTPScalingSet{}, f.defaultInformer)
}

func (f *clusterHTTPScalingSetInformer) Lister() v1alpha1.ClusterHTTPScalingSetLister {
	return v1alpha1.NewClusterHTTPScalingSetLister(f.Informer().GetIndexer())
}