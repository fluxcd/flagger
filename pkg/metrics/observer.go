package metrics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Observer is used to query Prometheus
type Observer struct {
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

// NewObserver creates a new observer
func NewObserver(metricsServer string) Observer {
	return Observer{
		metricsServer: metricsServer,
	}
}

// GetMetricsServer returns the Prometheus URL
func (c *Observer) GetMetricsServer() string {
	return c.metricsServer
}

func (c *Observer) queryMetric(query string) (*vectorQueryResponse, error) {
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
func (c *Observer) GetScalar(query string) (float64, error) {
	if c.metricsServer == "fake" {
		return 100, nil
	}

	query = strings.Replace(query, "\n", "", -1)
	query = strings.Replace(query, " ", "", -1)

	var value *float64

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
			value = &f
		}
	}
	if value == nil {
		return 0, fmt.Errorf("no values found for query %s", query)
	}
	return *value, nil
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

func render(meta interface{}, tmpl string) (string, error) {
	t, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var data bytes.Buffer
	b := bufio.NewWriter(&data)

	if err := t.Execute(b, meta); err != nil {
		return "", err
	}
	err = b.Flush()
	if err != nil {
		return "", err
	}

	res := strings.ReplaceAll(data.String(), "\n", "")

	return res, nil
}
