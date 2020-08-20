package observers

import (
	"strings"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

type Factory struct {
	Client providers.Interface
}

func NewFactory(metricsServer string) (*Factory, error) {
	client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
		Type:      "prometheus",
		Address:   metricsServer,
		SecretRef: nil,
	}, nil)
	if err != nil {
		return nil, err
	}

	return &Factory{
		Client: client,
	}, nil
}

func (factory Factory) Observer(provider string) Interface {
	switch {
	case strings.HasPrefix(provider, flaggerv1.AppMeshProvider):
		return &AppMeshObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.LinkerdProvider:
		return &LinkerdObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.IstioProvider:
		return &IstioObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.ContourProvider:
		return &ContourObserver{
			client: factory.Client,
		}
	case strings.HasPrefix(provider, flaggerv1.GlooProvider):
		return &GlooObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.NGINXProvider:
		return &NginxObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.KubernetesProvider:
		return &HttpObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.SkipperProvider:
		return &SkipperObserver{
			client: factory.Client,
		}
	default:
		return &IstioObserver{
			client: factory.Client,
		}
	}
}
