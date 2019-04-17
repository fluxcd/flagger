package router

import (
	"context"

	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

type Factory struct {
	kubeConfig    *restclient.Config
	kubeClient    kubernetes.Interface
	meshClient    clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func NewFactory(kubeConfig *restclient.Config, kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	logger *zap.SugaredLogger,
	meshClient clientset.Interface) *Factory {
	return &Factory{
		kubeConfig:    kubeConfig,
		meshClient:    meshClient,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		logger:        logger,
	}
}

// KubernetesRouter returns a ClusterIP service router
func (factory *Factory) KubernetesRouter(label string) *KubernetesRouter {
	return &KubernetesRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
		label:         label,
	}
}

// MeshRouter returns a service mesh router (Istio or AppMesh)
func (factory *Factory) MeshRouter(provider string) Interface {
	switch provider {
	case "appmesh":
		return &AppMeshRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
		}
	case "supergloo":
		supergloo, err := NewSuperglooRouter(context.TODO(), factory.flaggerClient, factory.logger, factory.kubeConfig)
		if err != nil {
			panic("TODO")
		}
		return supergloo
	default:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
		}
	}
}
