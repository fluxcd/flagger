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
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// PrometheusClient is executing promql queries
type PrometheusClient struct {
	timeout time.Duration
	url     url.URL
}

type prometheusResponse struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name string `json:"name"`
			}
			Value []interface{} `json:"value"`
		}
	}
}

// NewPrometheusClient creates a Prometheus client for the provided URL address
func NewPrometheusClient(address string, timeout time.Duration) (*PrometheusClient, error) {
	promURL, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	return &PrometheusClient{timeout: timeout, url: *promURL}, nil
}

// RenderQuery renders the promql query using the provided text template
func (p *PrometheusClient) RenderQuery(name string, namespace string, interval string, tmpl string) (string, error) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		name,
		namespace,
		interval,
	}

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

	return data.String(), nil
}

// RunQuery executes the promql and converts the result to float64
func (p *PrometheusClient) RunQuery(query string) (float64, error) {
	if p.url.Host == "fake" {
		return 100, nil
	}

	query = url.QueryEscape(p.TrimQuery(query))
	u, err := url.Parse(fmt.Sprintf("./api/v1/query?query=%s", query))
	if err != nil {
		return 0, err
	}
	u.Path = path.Join(p.url.Path, u.Path)

	u = p.url.ResolveReference(u)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()

	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return 0, err
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading body: %s", err.Error())
	}

	if 400 <= r.StatusCode {
		return 0, fmt.Errorf("error response: %s", string(b))
	}

	var result prometheusResponse
	err = json.Unmarshal(b, &result)
	if err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %s, '%s'", err.Error(), string(b))
	}

	var value *float64
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
		return 0, fmt.Errorf("no values found")
	}

	return *value, nil
}

// TrimQuery takes a promql query and removes spaces, tabs and new lines
func (p *PrometheusClient) TrimQuery(query string) string {
	query = strings.Replace(query, "\n", "", -1)
	query = strings.Replace(query, "\t", "", -1)
	query = strings.Replace(query, " ", "", -1)

	return query
}

// IsOnline call Prometheus status endpoint and returns an error if the API is unreachable
func (p *PrometheusClient) IsOnline() (bool, error) {
	u, err := url.Parse("./api/v1/status/flags")
	if err != nil {
		return false, err
	}
	u.Path = path.Join(p.url.Path, u.Path)

	u = p.url.ResolveReference(u)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
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

func (p *PrometheusClient) GetMetricsServer() string {
	return p.url.String()
}
