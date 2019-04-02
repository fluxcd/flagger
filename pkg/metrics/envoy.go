package metrics

import (
	"fmt"
	"net/url"
	"strconv"
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
