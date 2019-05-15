package metrics

import (
	"time"
)

var istioQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			istio_requests_total{
				reporter="destination",
				destination_workload_namespace="{{ .Namespace }}",
				destination_workload=~"{{ .Name }}",
				response_code!~"5.*"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			istio_requests_total{
				reporter="destination",
				destination_workload_namespace="{{ .Namespace }}",
				destination_workload=~"{{ .Name }}"
			}[{{ .Interval }}]
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
					destination_workload_namespace="{{ .Namespace }}",
					destination_workload=~"{{ .Name }}"
				}[{{ .Interval }}]
			)
		) by (le)
	)`,
}

type IstioObserver struct {
	client *PrometheusClient
}

func (ob *IstioObserver) GetRequestSuccessRate(name string, namespace string, interval string) (float64, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, istioQueries["request-success-rate"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *IstioObserver) GetRequestDuration(name string, namespace string, interval string) (time.Duration, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, istioQueries["request-duration"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	ms := time.Duration(int64(value*1000)) * time.Millisecond
	return ms, nil
}
