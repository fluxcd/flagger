package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CanaryObserver is used to query the Istio Prometheus db
type CanaryObserver struct {
	metricsServer string
}

type vectorQueryResponse struct {
	Data struct {
		Result []struct {
			Metric struct {
				Code string `json:"response_code"`
				Name string `json:"destination_workload"`
			}
			Value []interface{} `json:"value"`
		}
	}
}

func (c *CanaryObserver) queryMetric(query string) (*vectorQueryResponse, error) {
	promURL, err := url.Parse(c.metricsServer)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("./api/v1/query?query=%s", query))
	if err != nil {
		return nil, err
	}

	u = promURL.ResolveReference(u)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %s", err.Error())
	}

	if 400 <= r.StatusCode {
		return nil, fmt.Errorf("error response: %s", string(b))
	}

	var values vectorQueryResponse
	err = json.Unmarshal(b, &values)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling result: %s, '%s'", err.Error(), string(b))
	}

	return &values, nil
}

// GetScalar runs the promql query and returns the first value found
func (c *CanaryObserver) GetScalar(query string) (float64, error) {
	if c.metricsServer == "fake" {
		return 100, nil
	}

	query = strings.Replace(query, "\n", "", -1)
	query = strings.Replace(query, " ", "", -1)

	var value *float64
	result, err := c.queryMetric(query)
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
			value = &f
		}
	}
	if value == nil {
		return 0, fmt.Errorf("no values found for query %s", query)
	}
	return *value, nil
}

// GetDeploymentCounter returns the requests success rate using istio_requests_total metric
func (c *CanaryObserver) GetDeploymentCounter(name string, namespace string, metric string, interval string) (float64, error) {
	if c.metricsServer == "fake" {
		return 100, nil
	}

	var rate *float64
	querySt := url.QueryEscape(`sum(rate(` +
		metric + `{reporter="destination",destination_workload_namespace=~"` +
		namespace + `",destination_workload=~"` +
		name + `",response_code!~"5.*"}[1m])) / sum(rate(` +
		metric + `{reporter="destination",destination_workload_namespace=~"` +
		namespace + `",destination_workload=~"` +
		name + `"}[` +
		interval + `])) * 100 `)
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

// GetDeploymentHistogram returns the 99P requests delay using istio_request_duration_seconds_bucket metrics
func (c *CanaryObserver) GetDeploymentHistogram(name string, namespace string, metric string, interval string) (time.Duration, error) {
	if c.metricsServer == "fake" {
		return 1, nil
	}
	var rate *float64
	querySt := url.QueryEscape(`histogram_quantile(0.99, sum(rate(` +
		metric + `{reporter="destination",destination_workload=~"` +
		name + `", destination_workload_namespace=~"` +
		namespace + `"}[` +
		interval + `])) by (le))`)
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

// CheckMetricsServer call Prometheus status endpoint and returns an error if
// the API is unreachable
func CheckMetricsServer(address string) (bool, error) {
	promURL, err := url.Parse(address)
	if err != nil {
		return false, err
	}

	u, err := url.Parse("./api/v1/status/flags")
	if err != nil {
		return false, err
	}

	u = promURL.ResolveReference(u)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return false, err
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return false, fmt.Errorf("error reading body: %s", err.Error())
	}

	if 400 <= r.StatusCode {
		return false, fmt.Errorf("error response: %s", string(b))
	}

	return true, nil
}
