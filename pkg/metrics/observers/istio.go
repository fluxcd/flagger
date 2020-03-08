package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

var istioQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			istio_requests_total{
				reporter="destination",
				destination_workload_namespace="{{ namespace }}",
				destination_workload=~"{{ target }}",
				response_code!~"5.*"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			istio_requests_total{
				reporter="destination",
				destination_workload_namespace="{{ namespace }}",
				destination_workload=~"{{ target }}"
			}[{{ interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				istio_request_duration_seconds_bucket{
					reporter="destination",
					destination_workload_namespace="{{ namespace }}",
					destination_workload=~"{{ target }}"
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type IstioObserver struct {
	client providers.Interface
}

func (ob *IstioObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(istioQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *IstioObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(istioQueries["request-duration"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	ms := time.Duration(int64(value*1000)) * time.Millisecond
	return ms, nil
}
