package metrics

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const istioSuccessRateQuery = `
sum(rate(
istio_requests_total{reporter="destination",
destination_workload_namespace="{{ .Namespace }}",
destination_workload=~"{{ .Name }}",
response_code!~"5.*"}
[{{ .Interval }}])) 
/ 
sum(rate(
istio_requests_total{reporter="destination",
destination_workload_namespace="{{ .Namespace }}",
destination_workload=~"{{ .Name }}"}
[{{ .Interval }}])) 
* 100
`

// GetIstioSuccessRate returns the requests success rate (non 5xx) using istio_requests_total metric
func (c *Observer) GetIstioSuccessRate(name string, namespace string, metric string, interval string) (float64, error) {
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

	query, err := render(meta, istioSuccessRateQuery)
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

const istioRequestDurationQuery = `
histogram_quantile(0.99, sum(rate(
istio_request_duration_seconds_bucket{reporter="destination",
destination_workload_namespace="{{ .Namespace }}",
destination_workload=~"{{ .Name }}"}
[{{ .Interval }}])) by (le))
`

// GetIstioRequestDuration returns the 99P requests delay using istio_request_duration_seconds_bucket metrics
func (c *Observer) GetIstioRequestDuration(name string, namespace string, metric string, interval string) (time.Duration, error) {
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

	query, err := render(meta, istioRequestDurationQuery)
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
	ms := time.Duration(int64(*rate*1000)) * time.Millisecond
	return ms, nil
}
