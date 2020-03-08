package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

//envoy_cluster_name="test_podinfo-canary_9898"

var contourQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			envoy_cluster_upstream_rq{
				envoy_cluster_name=~"{{ namespace }}_{{ target }}-canary_[0-9a-zA-Z-]+",
				envoy_response_code!~"5.*"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			envoy_cluster_upstream_rq{
				envoy_cluster_name=~"{{ namespace }}_{{ target }}-canary_[0-9a-zA-Z-]+",
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
					envoy_cluster_name=~"{{ namespace }}_{{ target }}-canary_[0-9a-zA-Z-]+",
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type ContourObserver struct {
	client providers.Interface
}

func (ob *ContourObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(contourQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *ContourObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(contourQueries["request-duration"], model)
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
