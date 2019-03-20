package router

import (
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

type Factory struct {
	kubeClient    kubernetes.Interface
	meshClient    clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func NewFactory(kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	logger *zap.SugaredLogger,
	meshClient clientset.Interface) *Factory {
	return &Factory{
		meshClient:    meshClient,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		logger:        logger,
	}
}

// KubernetesRouter returns a ClusterIP service router
func (factory *Factory) KubernetesRouter() *KubernetesRouter {
	return &KubernetesRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
	}
}

// MeshRouter returns a service mesh router (Istio or AppMesh)
func (factory *Factory) MeshRouter(provider string) Interface {
	if provider == "appmesh" {
		return &AppMeshRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
		}
	}
	return &IstioRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
		istioClient:   factory.meshClient,
	}
}
