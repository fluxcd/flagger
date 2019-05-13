package metrics

import (
	"time"
)

var linkerdQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			response_total{
				namespace="{{ .Namespace }}",
				dst_deployment=~"{{ .Name }}",
				classification="failure"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			response_total{
				namespace="{{ .Namespace }}",
				dst_deployment=~"{{ .Name }}"
			}[{{ .Interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				response_latency_ms_bucket{
					namespace="{{ .Namespace }}",
					dst_deployment=~"{{ .Name }}"
				}[{{ .Interval }}]
			)
		) by (le)
	)`,
}

type LinkerdObserver struct {
	client *PrometheusClient
}

func (ob *LinkerdObserver) GetRequestSuccessRate(name string, namespace string, interval string) (float64, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, linkerdQueries["request-success-rate"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *LinkerdObserver) GetRequestDuration(name string, namespace string, interval string) (time.Duration, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, linkerdQueries["request-duration"])
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
