package metrics

import (
	"time"
)

var crossoverQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			envoy_cluster_upstream_rq{
				kubernetes_namespace="{{ .Namespace }}",
				envoy_cluster_name=~"{{ .Name }}-canary",
				envoy_response_code!~"5.*"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			envoy_cluster_upstream_rq{
				kubernetes_namespace="{{ .Namespace }}",
				envoy_cluster_name=~"{{ .Name }}-canary"
			}[{{ .Interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				envoy_cluster_upstream_rq_time_bucket{
					kubernetes_namespace="{{ .Namespace }}",
					envoy_cluster_name=~"{{ .Name }}-canary"
				}[{{ .Interval }}]
			)
		) by (le)
	)`,
}

type CrossoverObserver struct {
	client *PrometheusClient
}

func (ob *CrossoverObserver) GetRequestSuccessRate(name string, namespace string, interval string) (float64, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, crossoverQueries["request-success-rate"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *CrossoverObserver) GetRequestDuration(name string, namespace string, interval string) (time.Duration, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, crossoverQueries["request-duration"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	ms := time.Duration(int64(value)) * time.Millisecond
	return ms, nil
}
