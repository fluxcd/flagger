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

package router

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// MultiClusterClient holds both clients for a remote cluster
type MultiClusterClient struct {
	IstioClient clientset.Interface
	KubeClient  kubernetes.Interface
}

// ClusterManager manages Istio clients for multiple clusters discovered via Secrets
type ClusterManager struct {
	kubeClient      kubernetes.Interface
	meshClient      clientset.Interface
	logger          *zap.SugaredLogger
	secretLabel     string
	secretNamespace string
	clients         *sync.Map
}

// NewClusterManager returns a new ClusterManager
func NewClusterManager(
	kubeClient kubernetes.Interface,
	meshClient clientset.Interface,
	logger *zap.SugaredLogger,
	secretLabel string,
	secretNamespace string,
) *ClusterManager {
	return &ClusterManager{
		kubeClient:      kubeClient,
		meshClient:      meshClient,
		logger:          logger,
		secretLabel:     secretLabel,
		secretNamespace: secretNamespace,
		clients:         new(sync.Map),
	}
}

// Start watching for multi-cluster secrets
func (cm *ClusterManager) Start(stopCh <-chan struct{}) {
	cm.logger.Infof("Starting multi-cluster manager, watching for secrets with label %s in namespace %s", cm.secretLabel, cm.secretNamespace)

	watchlist := cache.NewFilteredListWatchFromClient(
		cm.kubeClient.CoreV1().RESTClient(),
		"secrets",
		cm.secretNamespace,
		func(options *metav1.ListOptions) {
			options.LabelSelector = cm.secretLabel
		},
	)

	_, informer := cache.NewInformer(
		watchlist,
		&corev1.Secret{},
		time.Minute*5,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				secret := obj.(*corev1.Secret)
				cm.addCluster(secret)
			},
			UpdateFunc: func(old, new interface{}) {
				secret := new.(*corev1.Secret)
				cm.addCluster(secret)
			},
			DeleteFunc: func(obj interface{}) {
				secret := obj.(*corev1.Secret)
				cm.removeCluster(secret)
			},
		},
	)

	go informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, informer.HasSynced) {
		cm.logger.Error("Timed out waiting for multi-cluster secrets cache to sync")
	}
}

func (cm *ClusterManager) addCluster(secret *corev1.Secret) {
	for name, data := range secret.Data {
		// Istio remote secrets contain the kubeconfig in a key that is the cluster name
		// We skip keys like 'token' as they are not kubeconfigs
		if name == "token" {
			continue
		}

		cm.logger.Infof("Multi-cluster manager: found cluster %s in secret %s", name, secret.Name)

		config, err := clientcmd.RESTConfigFromKubeConfig(data)
		if err != nil {
			cm.logger.Errorf("Multi-cluster manager: error building kubeconfig for cluster %s: %v", name, err)
			continue
		}

		client, err := clientset.NewForConfig(config)
		if err != nil {
			cm.logger.Errorf("Multi-cluster manager: error building Istio clientset for cluster %s: %v", name, err)
			continue
		}

		kubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			cm.logger.Errorf("Multi-cluster manager: error building kubernetes clientset for cluster %s: %v", name, err)
			continue
		}

		cm.clients.Store(name, &MultiClusterClient{
			IstioClient: client,
			KubeClient:  kubeClient,
		})
	}
}

func (cm *ClusterManager) removeCluster(secret *corev1.Secret) {
	for name := range secret.Data {
		if name == "token" {
			continue
		}
		cm.logger.Infof("Multi-cluster manager: removing cluster %s", name)
		cm.clients.Delete(name)
	}
}

// GetClients returns all Istio clients including the local one
func (cm *ClusterManager) GetClients() []clientset.Interface {
	var clients []clientset.Interface
	clients = append(clients, cm.meshClient)

	cm.clients.Range(func(key, value interface{}) bool {
		rc := value.(*MultiClusterClient)
		clients = append(clients, rc.IstioClient)
		return true
	})

	return clients
}

// GetMultiClusterClients returns all cluster clients (Istio + Kube) including the local one
func (cm *ClusterManager) GetMultiClusterClients() []MultiClusterClient {
	var clients []MultiClusterClient
	clients = append(clients, MultiClusterClient{
		IstioClient: cm.meshClient,
		KubeClient:  cm.kubeClient,
	})

	cm.clients.Range(func(key, value interface{}) bool {
		rc := value.(*MultiClusterClient)
		clients = append(clients, *rc)
		return true
	})

	return clients
}

// ResolveNamespace checks if a namespace exists on a remote cluster.
// If it does not exist, it creates it with istio-injection=enabled label.
func (cm *ClusterManager) ResolveNamespace(cluster MultiClusterClient, namespace string) string {
	// For the local cluster, always use the canary namespace
	if cluster.IstioClient == cm.meshClient {
		return namespace
	}

	kubeClient := cluster.KubeClient
	if kubeClient == nil {
		cm.logger.Warnf("Multi-cluster: could not find kube client for remote cluster, using namespace %s", namespace)
		return namespace
	}

	_, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		cm.logger.Infof("Multi-cluster: namespace %s not found on remote cluster, creating it", namespace)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Labels: map[string]string{
					"istio-injection": "enabled",
				},
			},
		}
		_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
		if err != nil {
			cm.logger.Errorf("Multi-cluster: failed to create namespace %s on remote cluster: %v", namespace, err)
			return namespace
		}
	}

	return namespace
}
