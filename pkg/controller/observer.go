package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type VectorQueryResponse struct {
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

func (c *Controller) queryMetric(query string) (*VectorQueryResponse, error) {
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

	var values VectorQueryResponse
	err = json.Unmarshal(b, &values)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling result: %s, '%s'", err.Error(), string(b))
	}

	return &values, nil
}

func (c *Controller) getDeploymentMetric(name string, namespace string, counter string, interval string) (float64, error) {
	var rate float64
	querySt := url.QueryEscape(`sum(rate(` +
		counter + `{reporter="destination",destination_workload_namespace=~"` +
		namespace + `",destination_workload=~"` +
		name + `",response_code!~"5.*"}[1m])) / sum(rate(` +
		counter + `{reporter="destination",destination_workload_namespace=~"` +
		namespace + `",destination_workload=~"` +
		name + `"}[` +
		interval + `])) * 100 `)
	result, err := c.queryMetric(querySt)
	if err != nil {
		return rate, err
	}

	for _, v := range result.Data.Result {
		metricValue := v.Value[1]
		switch metricValue.(type) {
		case string:
			f, err := strconv.ParseFloat(metricValue.(string), 64)
			if err != nil {
				return rate, err
			}
			rate = f
		}
	}
	return rate, nil
}

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
