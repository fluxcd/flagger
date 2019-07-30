package metrics

import (
	"strings"
	"time"
)

type Factory struct {
	MeshProvider string
	Client       *PrometheusClient
}

func NewFactory(metricsServer string, meshProvider string, timeout time.Duration) (*Factory, error) {
	client, err := NewPrometheusClient(metricsServer, timeout)
	if err != nil {
		return nil, err
	}

	return &Factory{
		MeshProvider: meshProvider,
		Client:       client,
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
		return &EnvoyObserver{
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
	case provider == "linkerd":
		return &LinkerdObserver{
			client: factory.Client,
		}
	default:
		return &IstioObserver{
			client: factory.Client,
		}
	}
}
