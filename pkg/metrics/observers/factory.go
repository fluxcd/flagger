/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package observers

import (
	"strings"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/metrics/providers"
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
	case provider == flaggerv1.TraefikProvider:
		return &TraefikObserver{
			client: factory.Client,
		}
	case provider == flaggerv1.OsmProvider:
		return &OsmObserver{
			client: factory.Client,
		}
	default:
		return &IstioObserver{
			client: factory.Client,
		}
	}
}
