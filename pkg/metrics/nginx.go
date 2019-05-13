package metrics

import (
	"time"
)

var nginxQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			nginx_ingress_controller_requests{
				namespace="{{ .Namespace }}",
				ingress="{{ .Name }}",
				status!~"5.*"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			nginx_ingress_controller_requests{
				namespace="{{ .Namespace }}",
				ingress="{{ .Name }}"
			}[{{ .Interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	sum(
		rate(
			nginx_ingress_controller_ingress_upstream_latency_seconds_sum{
				namespace="{{ .Namespace }}",
				ingress="{{ .Name }}"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			nginx_ingress_controller_ingress_upstream_latency_seconds_count{
				namespace="{{ .Namespace }}",
				ingress="{{ .Name }}"
			}[{{ .Interval }}]
		)
	) 
	* 1000`,
}

type NginxObserver struct {
	client *PrometheusClient
}

func (ob *NginxObserver) GetRequestSuccessRate(name string, namespace string, interval string) (float64, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, nginxQueries["request-success-rate"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *NginxObserver) GetRequestDuration(name string, namespace string, interval string) (time.Duration, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, nginxQueries["request-duration"])
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
