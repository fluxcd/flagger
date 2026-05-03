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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
)

func TestClusterManager_AddRemoveCluster(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	kubeClient := fake.NewSimpleClientset()
	meshClient := fakeFlagger.NewSimpleClientset()

	cm := NewClusterManager(kubeClient, meshClient, sugar, "istio/multiCluster=true", "istio-system")

	// Minimal kubeconfig that satisfies parsing
	kubeconfig := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: remote-cluster
contexts:
- context:
    cluster: remote-cluster
    user: remote-user
  name: remote-cluster
current-context: remote-cluster
kind: Config
users:
- name: remote-user
  user:
    token: fake-token
`
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "istio-remote-secret",
			Namespace: "istio-system",
			Labels: map[string]string{
				"istio/multiCluster": "true",
			},
		},
		Data: map[string][]byte{
			"remote-cluster": []byte(kubeconfig),
		},
	}

	// Before adding: only local client
	clients := cm.GetClients()
	assert.Len(t, clients, 1, "Should have 1 client (local) before adding")

	// Add remote cluster
	cm.addCluster(secret)
	clients = cm.GetClients()
	assert.Len(t, clients, 2, "Should have 2 clients (local + remote) after adding")

	// Re-add same cluster (should update, not duplicate)
	cm.addCluster(secret)
	clients = cm.GetClients()
	assert.Len(t, clients, 2, "Should still have 2 clients after re-adding")

	// Remove remote cluster
	cm.removeCluster(secret)
	clients = cm.GetClients()
	assert.Len(t, clients, 1, "Should have 1 client (local) after removal")
}

func TestClusterManager_ResolveNamespace(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	kubeClient := fake.NewSimpleClientset()
	meshClient := fakeFlagger.NewSimpleClientset()

	cm := NewClusterManager(kubeClient, meshClient, sugar, "istio/multiCluster=true", "istio-system")

	// Local cluster always resolves to original namespace
	assert.Equal(t, "original", cm.ResolveNamespace(MultiClusterClient{IstioClient: meshClient, KubeClient: kubeClient}, "original"))

	// Manually inject a remote cluster with fake clients to avoid panic and allow state control
	remoteMeshClient := fakeFlagger.NewSimpleClientset()
	remoteKubeClient := fake.NewSimpleClientset()
	cm.clients.Store("remote-cluster", &MultiClusterClient{
		IstioClient: remoteMeshClient,
		KubeClient:  remoteKubeClient,
	})

	// Get the injected client
	remoteClient := remoteMeshClient

	// Test creation when namespace doesn't exist
	assert.Equal(t, "non-existent", cm.ResolveNamespace(MultiClusterClient{IstioClient: remoteClient, KubeClient: remoteKubeClient}, "non-existent"))
	ns, err := remoteKubeClient.CoreV1().Namespaces().Get(context.TODO(), "non-existent", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "enabled", ns.Labels["istio-injection"])

	// Test success when namespace exists
	_, err = remoteKubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "existent"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "existent", cm.ResolveNamespace(MultiClusterClient{IstioClient: remoteClient, KubeClient: remoteKubeClient}, "existent"))
}

func TestClusterManager_SkipsTokenKey(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	kubeClient := fake.NewSimpleClientset()
	meshClient := fakeFlagger.NewSimpleClientset()

	cm := NewClusterManager(kubeClient, meshClient, sugar, "istio/multiCluster=true", "istio-system")

	// Secret with only a "token" key — should be skipped
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "istio-remote-secret-token-only",
			Namespace: "istio-system",
		},
		Data: map[string][]byte{
			"token": []byte("some-token"),
		},
	}

	cm.addCluster(secret)
	clients := cm.GetClients()
	assert.Len(t, clients, 1, "Should still have only 1 client (local) — token key is skipped")
}

func TestClusterManager_InvalidKubeconfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	kubeClient := fake.NewSimpleClientset()
	meshClient := fakeFlagger.NewSimpleClientset()

	cm := NewClusterManager(kubeClient, meshClient, sugar, "istio/multiCluster=true", "istio-system")

	// Secret with invalid kubeconfig data
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-secret",
			Namespace: "istio-system",
		},
		Data: map[string][]byte{
			"remote-cluster": []byte("this is not a valid kubeconfig"),
		},
	}

	cm.addCluster(secret)
	clients := cm.GetClients()
	assert.Len(t, clients, 1, "Should still have only 1 client (local) — invalid kubeconfig is skipped")
}

func TestIstioRouter_FinalizeMultiCluster(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	kubeClient := fake.NewSimpleClientset()
	meshClient := fakeFlagger.NewSimpleClientset()

	cm := NewClusterManager(kubeClient, meshClient, sugar, "istio/multiCluster=true", "istio-system")

	// Inject a remote cluster
	remoteMeshClient := fakeFlagger.NewSimpleClientset()
	remoteKubeClient := fake.NewSimpleClientset()
	cm.clients.Store("remote-cluster", &MultiClusterClient{
		IstioClient: remoteMeshClient,
		KubeClient:  remoteKubeClient,
	})

	router := &IstioRouter{
		logger:         sugar,
		kubeClient:     kubeClient,
		istioClients:   []clientset.Interface{meshClient},
		clusterManager: cm,
	}

	canary := newTestCanary()

	// 1. Setup local cluster VirtualService (with annotation so it reverts)
	vs := &istiov1beta1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name,
			Namespace: canary.Namespace,
			Annotations: map[string]string{
				kubectlAnnotation: "{}",
			},
		},
	}
	_, err := meshClient.NetworkingV1beta1().VirtualServices(canary.Namespace).Create(context.TODO(), vs, metav1.CreateOptions{})
	require.NoError(t, err)

	// 2. Setup remote cluster resources (replicated)
	// DestinationRules (created by Flagger)
	drPrimary := &istiov1beta1.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name + "-primary",
			Namespace: canary.Namespace,
		},
	}
	drCanary := &istiov1beta1.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name + "-canary",
			Namespace: canary.Namespace,
		},
	}
	_, err = remoteMeshClient.NetworkingV1beta1().DestinationRules(canary.Namespace).Create(context.TODO(), drPrimary, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = remoteMeshClient.NetworkingV1beta1().DestinationRules(canary.Namespace).Create(context.TODO(), drCanary, metav1.CreateOptions{})
	require.NoError(t, err)

	// VirtualService (created by Flagger, no orig-config)
	remoteVS := &istiov1beta1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canary.Name,
			Namespace: canary.Namespace,
		},
	}
	_, err = remoteMeshClient.NetworkingV1beta1().VirtualServices(canary.Namespace).Create(context.TODO(), remoteVS, metav1.CreateOptions{})
	require.NoError(t, err)

	// 3. Finalize
	err = router.Finalize(canary)
	require.NoError(t, err)

	// 4. Verify Local: VirtualService should still exist (reverted)
	_, err = meshClient.NetworkingV1beta1().VirtualServices(canary.Namespace).Get(context.TODO(), canary.Name, metav1.GetOptions{})
	assert.NoError(t, err, "Local VirtualService should still exist")

	// 5. Verify Remote: DestinationRules should be DELETED
	_, err = remoteMeshClient.NetworkingV1beta1().DestinationRules(canary.Namespace).Get(context.TODO(), canary.Name+"-primary", metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err), "Remote primary DestinationRule should be deleted")
	_, err = remoteMeshClient.NetworkingV1beta1().DestinationRules(canary.Namespace).Get(context.TODO(), canary.Name+"-canary", metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err), "Remote canary DestinationRule should be deleted")

	// 6. Verify Remote: VirtualService should be DELETED (no orig-config)
	_, err = remoteMeshClient.NetworkingV1beta1().VirtualServices(canary.Namespace).Get(context.TODO(), canary.Name, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err), "Remote VirtualService should be deleted")
}
