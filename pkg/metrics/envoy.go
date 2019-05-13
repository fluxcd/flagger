package metrics

import (
	"time"
)

var envoyQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			envoy_cluster_upstream_rq{
				kubernetes_namespace="{{ .Namespace }}",
				kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
				envoy_response_code!~"5.*"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			envoy_cluster_upstream_rq{
				kubernetes_namespace="{{ .Namespace }}",
				kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
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
					kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
				}[{{ .Interval }}]
			)
		) by (le)
	)`,
}

type EnvoyObserver struct {
	client *PrometheusClient
}

func (ob *EnvoyObserver) GetRequestSuccessRate(name string, namespace string, interval string) (float64, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, envoyQueries["request-success-rate"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *EnvoyObserver) GetRequestDuration(name string, namespace string, interval string) (time.Duration, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, envoyQueries["request-duration"])
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
