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

package providers

import (
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	rest "k8s.io/client-go/rest"
)

type Factory struct{}

func (factory Factory) Provider(metricInterval string, provider flaggerv1.MetricTemplateProvider, credentials map[string][]byte, config *rest.Config) (Interface, error) {
	switch provider.Type {
	case "prometheus":
		return NewPrometheusProvider(provider, credentials)
	case "datadog":
		return NewDatadogProvider(metricInterval, provider, credentials)
	case "cloudwatch":
		return NewCloudWatchProvider(metricInterval, provider)
	case "newrelic":
		return NewNewRelicProvider(metricInterval, provider, credentials)
	case "graphite":
		return NewGraphiteProvider(provider, credentials)
	case "stackdriver":
		return NewStackDriverProvider(provider, credentials)
	case "influxdb":
		return NewInfluxdbProvider(provider, credentials)
	case "dynatrace":
		return NewDynatraceProvider(metricInterval, provider, credentials)
	case "keptn":
		return NewKeptnProvider(config)
	case "splunk":
		return NewSplunkProvider(metricInterval, provider, credentials)
	default:
		return NewPrometheusProvider(provider, credentials)
	}
}
