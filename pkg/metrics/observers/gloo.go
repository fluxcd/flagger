package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

//envoy_cluster_name="test-podinfo-primary-9898_gloo-system"

var glooQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			envoy_cluster_upstream_rq{
				envoy_cluster_name=~"{{ namespace }}-{{ target }}-canary-[0-9a-zA-Z-]+_[0-9a-zA-Z-]+",
				envoy_response_code!~"5.*"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			envoy_cluster_upstream_rq{
				envoy_cluster_name=~"{{ namespace }}-{{ target }}-canary-[0-9a-zA-Z-]+_[0-9a-zA-Z-]+",
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
					envoy_cluster_name=~"{{ namespace }}-{{ target }}-canary-[0-9a-zA-Z-]+_[0-9a-zA-Z-]+",
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type GlooObserver struct {
	client providers.Interface
}

func (ob *GlooObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(glooQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *GlooObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(glooQueries["request-duration"], model)
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
