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
				deployment=~"{{ .Name }}",
				classification!="failure",
				direction="inbound"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			response_total{
				namespace="{{ .Namespace }}",
				deployment=~"{{ .Name }}",
				direction="inbound"
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
					deployment=~"{{ .Name }}",
					direction="inbound"
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
