package router

import (
	"strings"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

type Factory struct {
	kubeConfig               *restclient.Config
	kubeClient               kubernetes.Interface
	meshClient               clientset.Interface
	flaggerClient            clientset.Interface
	ingressAnnotationsPrefix string
	ingressClass             string
	logger                   *zap.SugaredLogger
}

func NewFactory(kubeConfig *restclient.Config, kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	ingressAnnotationsPrefix string,
	ingressClass string,
	logger *zap.SugaredLogger,
	meshClient clientset.Interface) *Factory {
	return &Factory{
		kubeConfig:               kubeConfig,
		meshClient:               meshClient,
		kubeClient:               kubeClient,
		flaggerClient:            flaggerClient,
		ingressAnnotationsPrefix: ingressAnnotationsPrefix,
		ingressClass:             ingressClass,
		logger:                   logger,
	}
}

// KubernetesRouter returns a KubernetesRouter interface implementation
func (factory *Factory) KubernetesRouter(kind string, labelSelector string, ports map[string]int32) KubernetesRouter {
	switch kind {
	case "Service":
		return &KubernetesNoopRouter{}
	default: // Daemonset or Deployment
		return &KubernetesDefaultRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			labelSelector: labelSelector,
			ports:         ports,
		}
	}
}

// MeshRouter returns a service mesh router
func (factory *Factory) MeshRouter(provider string, labelSelector string) Interface {
	switch {
	case strings.HasPrefix(provider, flaggerv1.AppMeshProvider+":v1beta2"):
		return &AppMeshv1beta2Router{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
			labelSelector: labelSelector,
		}
	case provider == flaggerv1.AppMeshProvider:
		return &AppMeshRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
		}
	case provider == flaggerv1.LinkerdProvider:
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    flaggerv1.LinkerdProvider,
		}
	case provider == flaggerv1.IstioProvider:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
		}
	case strings.HasPrefix(provider, flaggerv1.SMIProvider):
		mesh := strings.TrimPrefix(provider, flaggerv1.SMIProvider+":")
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    mesh,
		}
	case provider == flaggerv1.ContourProvider:
		return &ContourRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			contourClient: factory.meshClient,
			ingressClass:  factory.ingressClass,
		}
	case strings.HasPrefix(provider, flaggerv1.GlooProvider):
		upstreamDiscoveryNs := flaggerv1.GlooProvider + "-system"
		if strings.HasPrefix(provider, flaggerv1.GlooProvider+":") {
			upstreamDiscoveryNs = strings.TrimPrefix(provider, flaggerv1.GlooProvider+":")
		}
		return &GlooRouter{
			logger:              factory.logger,
			flaggerClient:       factory.flaggerClient,
			kubeClient:          factory.kubeClient,
			glooClient:          factory.meshClient,
			upstreamDiscoveryNs: upstreamDiscoveryNs,
		}
	case provider == flaggerv1.NGINXProvider:
		return &IngressRouter{
			logger:            factory.logger,
			kubeClient:        factory.kubeClient,
			annotationsPrefix: factory.ingressAnnotationsPrefix,
		}
	case provider == flaggerv1.SkipperProvider:
		return &SkipperRouter{
			logger:     factory.logger,
			kubeClient: factory.kubeClient,
		}
	case provider == flaggerv1.KubernetesProvider:
		return &NopRouter{}
	default:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
		}
	}
}
