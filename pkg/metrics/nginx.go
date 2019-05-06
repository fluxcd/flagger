package metrics

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const nginxSuccessRateQuery = `
sum(rate(
nginx_ingress_controller_requests{kubernetes_namespace="{{ .Namespace }}",
ingress="{{ .Name }}",
status!~"5.*"}
[{{ .Interval }}])) 
/ 
sum(rate(
nginx_ingress_controller_requests{kubernetes_namespace="{{ .Namespace }}",
ingress="{{ .Name }}"}
[{{ .Interval }}])) 
* 100
`

// GetNginxSuccessRate returns the requests success rate (non 5xx) using nginx_ingress_controller_requests metric
func (c *Observer) GetNginxSuccessRate(name string, namespace string, metric string, interval string) (float64, error) {
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

	query, err := render(meta, nginxSuccessRateQuery)
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

const nginxRequestDurationQuery = `
sum(rate(
nginx_ingress_controller_ingress_upstream_latency_seconds_sum{kubernetes_namespace="{{ .Namespace }}",
ingress="{{ .Name }}"}[{{ .Interval }}])) 
/
sum(rate(nginx_ingress_controller_ingress_upstream_latency_seconds_count{kubernetes_namespace="{{ .Namespace }}",
ingress="{{ .Name }}"}[{{ .Interval }}])) * 1000
`

// GetNginxRequestDuration returns the avg requests latency using nginx_ingress_controller_ingress_upstream_latency_seconds_sum metric
func (c *Observer) GetNginxRequestDuration(name string, namespace string, metric string, interval string) (time.Duration, error) {
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

	query, err := render(meta, nginxRequestDurationQuery)
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
