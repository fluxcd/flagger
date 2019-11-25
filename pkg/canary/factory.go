package canary

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

type Factory struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker ConfigTracker
	labels        []string
}

func NewFactory(kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	configTracker ConfigTracker,
	labels []string,
	logger *zap.SugaredLogger) *Factory {
	return &Factory{
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		logger:        logger,
		configTracker: configTracker,
		labels:        labels,
	}
}

func (factory *Factory) Controller(kind string) Controller {
	deploymentCtrl := &DeploymentController{
		logger:        factory.logger,
		kubeClient:    factory.kubeClient,
		flaggerClient: factory.flaggerClient,
		labels:        factory.labels,
		configTracker: ConfigTracker{
			Logger:        factory.logger,
			KubeClient:    factory.kubeClient,
			FlaggerClient: factory.flaggerClient,
		},
	}

	switch {
	case kind == "Deployment":
		return deploymentCtrl
	default:
		return deploymentCtrl
	}

}
