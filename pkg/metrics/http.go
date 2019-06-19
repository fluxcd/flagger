package metrics

import "time"

var httpQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			http_request_duration_seconds_count{
				kubernetes_namespace="{{ .Namespace }}",
				kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
				status!~"5.*"
			}[{{ .Interval }}]
		)
	) 
	/ 
	sum(
		rate(
			http_request_duration_seconds_count{
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
				http_request_duration_seconds_bucket{
					kubernetes_namespace="{{ .Namespace }}",
					kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
				}[{{ .Interval }}]
			)
		) by (le)
	)`,
}

type HttpObserver struct {
	client *PrometheusClient
}

func (ob *HttpObserver) GetRequestSuccessRate(name string, namespace string, interval string) (float64, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, httpQueries["request-success-rate"])
	if err != nil {
		return 0, err
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (ob *HttpObserver) GetRequestDuration(name string, namespace string, interval string) (time.Duration, error) {
	query, err := ob.client.RenderQuery(name, namespace, interval, httpQueries["request-duration"])
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
