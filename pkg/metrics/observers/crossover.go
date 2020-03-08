package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

var crossoverQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			envoy_cluster_upstream_rq{
				kubernetes_namespace="{{ namespace }}",
				envoy_cluster_name=~"{{ target }}-canary",
				envoy_response_code!~"5.*"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			envoy_cluster_upstream_rq{
				kubernetes_namespace="{{ namespace }}",
				envoy_cluster_name=~"{{ target }}-canary"
			}[{{ interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				envoy_cluster_upstream_rq_time_bucket{
					kubernetes_namespace="{{ namespace }}",
					envoy_cluster_name=~"{{ target }}-canary"
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type CrossoverObserver struct {
	client providers.Interface
}

func (ob *CrossoverObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(crossoverQueries["request-success-rate"], model)
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *CrossoverObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(crossoverQueries["request-duration"], model)
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
