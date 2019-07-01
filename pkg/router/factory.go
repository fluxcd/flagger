package router

import (
	"context"
	"strings"

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
func (factory *Factory) KubernetesRouter(label string, ports *map[string]int32) *KubernetesRouter {
	return &KubernetesRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
		label:         label,
		ports:         ports,
	}
}

// MeshRouter returns a service mesh router
func (factory *Factory) MeshRouter(provider string) Interface {
	switch {
	case provider == "none":
		return &NopRouter{}
	case provider == "kubernetes":
		return &NopRouter{}
	case provider == "nginx":
		return &IngressRouter{
			logger:     factory.logger,
			kubeClient: factory.kubeClient,
		}
	case provider == "appmesh":
		return &AppMeshRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
		}
	case strings.HasPrefix(provider, "smi:"):
		mesh := strings.TrimPrefix(provider, "smi:")
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    mesh,
		}
	case provider == "linkerd":
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    "linkerd",
		}
	case strings.HasPrefix(provider, "supergloo"):
		supergloo, err := NewSuperglooRouter(context.TODO(), provider, factory.flaggerClient, factory.logger, factory.kubeConfig)
		if err != nil {
			panic("failed creating supergloo client")
		}
		return supergloo
	case strings.HasPrefix(provider, "gloo"):
		gloo, err := NewGlooRouter(context.TODO(), provider, factory.flaggerClient, factory.logger, factory.kubeConfig)
		if err != nil {
			panic("failed creating gloo client")
		}
		return gloo
	default:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
		}
	}
}
