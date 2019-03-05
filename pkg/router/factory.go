package router

import (
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

type Factory struct {
	kubeClient    kubernetes.Interface
	istioClient   istioclientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func NewFactory(kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	logger *zap.SugaredLogger,
	istioClient istioclientset.Interface) *Factory {
	return &Factory{
		istioClient:   istioClient,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		logger:        logger,
	}
}

func (factory *Factory) ServiceRouter() *ServiceRouter {
	return &ServiceRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
	}
}

func (factory *Factory) IstioRouter() *IstioRouter {
	return &IstioRouter{
		logger:        factory.logger,
		flaggerClient: factory.flaggerClient,
		kubeClient:    factory.kubeClient,
		istioClient:   factory.istioClient,
	}
}
