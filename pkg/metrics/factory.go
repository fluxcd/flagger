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

func (factory Factory) Observer() Interface {
	switch {
	case factory.MeshProvider == "none":
		return &HttpObserver{
			client: factory.Client,
		}
	case factory.MeshProvider == "appmesh":
		return &EnvoyObserver{
			client: factory.Client,
		}
	case factory.MeshProvider == "nginx":
		return &NginxObserver{
			client: factory.Client,
		}
	case strings.HasPrefix(factory.MeshProvider, "gloo"):
		return &GlooObserver{
			client: factory.Client,
		}
	case factory.MeshProvider == "smi:linkerd":
		return &LinkerdObserver{
			client: factory.Client,
		}
	default:
		return &IstioObserver{
			client: factory.Client,
		}
	}
}
