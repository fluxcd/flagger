package deployer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
)

type Observer struct {
	URL      *url.URL
	Interval string
}

func NewObserver(addr string, interval string) (*Observer, error) {
	promURL, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	return &Observer{
		URL:      promURL,
		Interval: interval,
	}, nil
}

func (c *Observer) queryRange(query string) (*VectorQueryResponse, error) {
	u, err := url.Parse(fmt.Sprintf("./api/v1/query?query=%s", query))
	if err != nil {
		return nil, err
	}

	u = c.URL.ResolveReference(u)
	r, err := http.Get(u.String())
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

type VectorQueryResponse struct {
	Data struct {
		Result []struct {
			Metric struct {
				Code         string `json:"code"`
				FunctionName string `json:"function_name"`
			}
			Value []interface{} `json:"value"`
		}
	}
}

func (c *Observer) GetDeploymentSuccessRate(namespace string, name string) (float64, error) {
	var rate float64
	querySt := url.QueryEscape(`sum(rate(istio_requests_total{reporter="destination",destination_workload_namespace=~"` + namespace + `",destination_workload=~"` + name + `",response_code!~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination",destination_workload_namespace=~"` + namespace + `",destination_workload=~"` + name + `"}[` + c.Interval + `])) * 100 `)
	result, err := c.queryRange(querySt)
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
