package cache

import (
	"context"
	"sync"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/controller"

	kubeinformers "k8s.io/client-go/informers"
	kubelisters "k8s.io/client-go/listers/core/v1"

	"k8s.io/client-go/kubernetes"
)

type KubeCoreCache interface {
	PodLister() kubelisters.PodLister
	ConfigMapLister() kubelisters.ConfigMapLister
	SecretLister() kubelisters.SecretLister
	NamespaceLister() kubelisters.NamespaceLister
	Subscribe() <-chan struct{}
	Unsubscribe(<-chan struct{})
}

type kubeCoreCaches struct {
	podLister       kubelisters.PodLister
	configMapLister kubelisters.ConfigMapLister
	secretLister    kubelisters.SecretLister
	namespaceLister kubelisters.NamespaceLister

	cacheUpdatedWatchers      []chan struct{}
	cacheUpdatedWatchersMutex sync.Mutex
}

// This context should live as long as the cache is desired. i.e. if the cache is shared
// across clients, it should get a context that has a longer lifetime than the clients themselves
func NewKubeCoreCache(ctx context.Context, client kubernetes.Interface) (*kubeCoreCaches, error) {
	resyncDuration := 12 * time.Hour
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(client, resyncDuration)

	pods := kubeInformerFactory.Core().V1().Pods()
	configMaps := kubeInformerFactory.Core().V1().ConfigMaps()
	secrets := kubeInformerFactory.Core().V1().Secrets()
	namespaces := kubeInformerFactory.Core().V1().Namespaces()

	k := &kubeCoreCaches{
		podLister:       pods.Lister(),
		configMapLister: configMaps.Lister(),
		secretLister:    secrets.Lister(),
		namespaceLister: namespaces.Lister(),
	}

	kubeController := controller.NewController("kube-plugin-controller",
		controller.NewLockingSyncHandler(k.updatedOccured),
		pods.Informer(),
		configMaps.Informer(),
		secrets.Informer(),
		namespaces.Informer(),
	)

	stop := ctx.Done()
	err := kubeController.Run(2, stop)
	if err != nil {
		return nil, err
	}

	return k, nil
}

func (k *kubeCoreCaches) PodLister() kubelisters.PodLister {
	return k.podLister
}

func (k *kubeCoreCaches) ConfigMapLister() kubelisters.ConfigMapLister {
	return k.configMapLister
}

func (k *kubeCoreCaches) SecretLister() kubelisters.SecretLister {
	return k.secretLister
}

func (k *kubeCoreCaches) NamespaceLister() kubelisters.NamespaceLister {
	return k.namespaceLister
}


func (k *kubeCoreCaches) Subscribe() <-chan struct{} {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	c := make(chan struct{}, 10)
	k.cacheUpdatedWatchers = append(k.cacheUpdatedWatchers, c)
	return c
}

func (k *kubeCoreCaches) Unsubscribe(c <-chan struct{}) {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	for i, cacheUpdated := range k.cacheUpdatedWatchers {
		if cacheUpdated == c {
			k.cacheUpdatedWatchers = append(k.cacheUpdatedWatchers[:i], k.cacheUpdatedWatchers[i+1:]...)
			return
		}
	}
}

func (k *kubeCoreCaches) updatedOccured() {
	k.cacheUpdatedWatchersMutex.Lock()
	defer k.cacheUpdatedWatchersMutex.Unlock()
	for _, cacheUpdated := range k.cacheUpdatedWatchers {
		select {
		case cacheUpdated <- struct{}{}:
		default:
		}
	}
}
