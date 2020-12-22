package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

var nginxQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			nginx_ingress_controller_requests{
				namespace="{{ namespace }}",
				ingress="{{ ingress }}",
				status!~"5.*"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			nginx_ingress_controller_requests{
				namespace="{{ namespace }}",
				ingress="{{ ingress }}"
			}[{{ interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	sum(
		rate(
			nginx_ingress_controller_ingress_upstream_latency_seconds_sum{
				namespace="{{ namespace }}",
				ingress="{{ ingress }}"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			nginx_ingress_controller_ingress_upstream_latency_seconds_count{
				namespace="{{ namespace }}",
				ingress="{{ ingress }}"
			}[{{ interval }}]
		)
	) 
	* 1000`,
}

type NginxObserver struct {
	client providers.Interface
}

func (ob *NginxObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(nginxQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *NginxObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(nginxQueries["request-duration"], model)
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
