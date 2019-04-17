package metrics

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const envoySuccessRateQuery = `
sum(rate(
envoy_cluster_upstream_rq{kubernetes_namespace="{{ .Namespace }}",
kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
envoy_response_code!~"5.*"}
[{{ .Interval }}])) 
/ 
sum(rate(
envoy_cluster_upstream_rq{kubernetes_namespace="{{ .Namespace }}",
kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"}
[{{ .Interval }}])) 
* 100
`

func (c *Observer) GetEnvoySuccessRate(name string, namespace string, metric string, interval string) (float64, error) {
	if c.metricsServer == "fake" {
		return 100, nil
	}

	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		name,
		namespace,
		interval,
	}

	query, err := render(meta, envoySuccessRateQuery)
	if err != nil {
		return 0, err
	}

	var rate *float64
	querySt := url.QueryEscape(query)
	result, err := c.queryMetric(querySt)
	if err != nil {
		return 0, err
	}

	for _, v := range result.Data.Result {
		metricValue := v.Value[1]
		switch metricValue.(type) {
		case string:
			f, err := strconv.ParseFloat(metricValue.(string), 64)
			if err != nil {
				return 0, err
			}
			rate = &f
		}
	}
	if rate == nil {
		return 0, fmt.Errorf("no values found for metric %s", metric)
	}
	return *rate, nil
}

const envoyRequestDurationQuery = `
histogram_quantile(0.99, sum(rate(
envoy_cluster_upstream_rq_time_bucket{kubernetes_namespace="{{ .Namespace }}",
kubernetes_pod_name=~"{{ .Name }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"}
[{{ .Interval }}])) by (le))
`

// GetEnvoyRequestDuration returns the 99P requests delay using envoy_cluster_upstream_rq_time_bucket metrics
func (c *Observer) GetEnvoyRequestDuration(name string, namespace string, metric string, interval string) (time.Duration, error) {
	if c.metricsServer == "fake" {
		return 1, nil
	}

	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		name,
		namespace,
		interval,
	}

	query, err := render(meta, envoyRequestDurationQuery)
	if err != nil {
		return 0, err
	}

	var rate *float64
	querySt := url.QueryEscape(query)
	result, err := c.queryMetric(querySt)
	if err != nil {
		return 0, err
	}

	for _, v := range result.Data.Result {
		metricValue := v.Value[1]
		switch metricValue.(type) {
		case string:
			f, err := strconv.ParseFloat(metricValue.(string), 64)
			if err != nil {
				return 0, err
			}
			rate = &f
		}
	}
	if rate == nil {
		return 0, fmt.Errorf("no values found for metric %s", metric)
	}
	ms := time.Duration(int64(*rate)) * time.Millisecond
	return ms, nil
}
