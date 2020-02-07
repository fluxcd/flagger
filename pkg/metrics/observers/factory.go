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
	case provider == "none":
		return &HttpObserver{
			client: factory.Client,
		}
	case provider == "kubernetes":
		return &HttpObserver{
			client: factory.Client,
		}
	case provider == "appmesh":
		return &AppMeshObserver{
			client: factory.Client,
		}
	case provider == "crossover":
		return &CrossoverObserver{
			client: factory.Client,
		}
	case provider == "nginx":
		return &NginxObserver{
			client: factory.Client,
		}
	case strings.HasPrefix(provider, "gloo"):
		return &GlooObserver{
			client: factory.Client,
		}
	case provider == "smi:linkerd":
		return &LinkerdObserver{
			client: factory.Client,
		}
	case provider == "crossover:service":
		return &CrossoverServiceObserver{
			client: factory.Client,
		}
	case provider == "linkerd":
		return &LinkerdObserver{
			client: factory.Client,
		}
	case provider == "contour":
		return &ContourObserver{
			client: factory.Client,
		}
	default:
		return &IstioObserver{
			client: factory.Client,
		}
	}
}
