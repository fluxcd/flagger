/*
Copyright 2022 The Flux authors

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
	"fmt"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/metrics/providers"
)

var apisixQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			apisix_http_status{
				route=~"{{ namespace }}_{{ target }}-canary_.+",
				code!~"5.."
			}[{{ interval }}]
		)
	)
	/
	sum(
		rate(
			apisix_http_status{
				route=~"{{ namespace }}_{{ target }}-canary_.+"
			}[{{ interval }}]
		)
	) * 100`,
	"request-duration": `
	histogram_quantile(
		0.99, 
		sum(
			rate(
				apisix_http_latency_bucket{
					type=~"request",
					route=~"{{ namespace }}_{{ target }}-canary_.+"
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type ApisixObserver struct {
	client providers.Interface
}

func (ob *ApisixObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {

	query, err := RenderQuery(apisixQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}
	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *ApisixObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(apisixQueries["request-duration"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	ms := time.Duration(int64(value)) * time.Millisecond
	return ms, nil
}
