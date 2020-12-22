package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

var httpQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			http_request_duration_seconds_count{
				kubernetes_namespace="{{ namespace }}",
				kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
				status!~"5.*"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			http_request_duration_seconds_count{
				kubernetes_namespace="{{ namespace }}",
				kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
			}[{{ interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				http_request_duration_seconds_bucket{
					kubernetes_namespace="{{ namespace }}",
					kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type HttpObserver struct {
	client providers.Interface
}

func (ob *HttpObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(httpQueries["request-success-rate"], model)
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *HttpObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(httpQueries["request-duration"], model)
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
