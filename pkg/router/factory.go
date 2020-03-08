package router

import (
	"strings"

	consulapi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

type Factory struct {
	kubeConfig               *restclient.Config
	kubeClient               kubernetes.Interface
	meshClient               clientset.Interface
	flaggerClient            clientset.Interface
	consulClient             *consulapi.Client
	ingressAnnotationsPrefix string
	logger                   *zap.SugaredLogger
}

func NewFactory(kubeConfig *restclient.Config, kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	ingressAnnotationsPrefix string,
	logger *zap.SugaredLogger,
	meshClient clientset.Interface,
	consulClient *consulapi.Client) *Factory {
	return &Factory{
		kubeConfig:               kubeConfig,
		meshClient:               meshClient,
		kubeClient:               kubeClient,
		flaggerClient:            flaggerClient,
		ingressAnnotationsPrefix: ingressAnnotationsPrefix,
		consulClient:             consulClient,
		logger:                   logger,
	}
}

// KubernetesDeploymentRouter returns a ClusterIP service router
func (factory *Factory) KubernetesRouter(kind string, labelSelector string, annotations map[string]string, ports map[string]int32) KubernetesRouter {
	deploymentRouter := &KubernetesDeploymentRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
		labelSelector: labelSelector,
		annotations:   annotations,
		ports:         ports,
	}
	noopRouter := &KubernetesNoopRouter{}

	switch {
	case kind == "Deployment":
		return deploymentRouter
	case kind == "Service":
		return noopRouter
	default:
		return deploymentRouter
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
			logger:            factory.logger,
			kubeClient:        factory.kubeClient,
			annotationsPrefix: factory.ingressAnnotationsPrefix,
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
	case provider == "contour":
		return &ContourRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			contourClient: factory.meshClient,
		}
	case strings.HasPrefix(provider, "gloo"):
		upstreamDiscoveryNs := "gloo-system"
		if strings.HasPrefix(provider, "gloo:") {
			upstreamDiscoveryNs = strings.TrimPrefix(provider, "gloo:")
		}
		return &GlooRouter{
			logger:              factory.logger,
			flaggerClient:       factory.flaggerClient,
			kubeClient:          factory.kubeClient,
			glooClient:          factory.meshClient,
			upstreamDiscoveryNs: upstreamDiscoveryNs,
		}
	case strings.HasPrefix(provider, "supergloo:appmesh"):
		return &AppMeshRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
		}
	case strings.HasPrefix(provider, "supergloo:istio"):
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
		}
	case strings.HasPrefix(provider, "supergloo:linkerd"):
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    "linkerd",
		}
	case provider == "connect" && factory.consulClient != nil:
		return &ConsulConnectRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			consulClient:  factory.consulClient,
		}
	default:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
		}
	}
}
