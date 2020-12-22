package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

const prometheusOnlineQuery = "vector(1)"

// PrometheusProvider executes promQL queries
type PrometheusProvider struct {
	timeout  time.Duration
	url      url.URL
	username string
	password string
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

// NewPrometheusProvider takes a provider spec and the credentials map,
// validates the address, extracts the username and password values if provided and
// returns a Prometheus client ready to execute queries against the API
func NewPrometheusProvider(provider flaggerv1.MetricTemplateProvider, credentials map[string][]byte) (*PrometheusProvider, error) {
	promURL, err := url.Parse(provider.Address)
	if provider.Address == "" || err != nil {
		return nil, fmt.Errorf("%s address %s is not a valid URL", provider.Type, provider.Address)
	}

	prom := PrometheusProvider{
		timeout: 5 * time.Second,
		url:     *promURL,
	}

	if provider.SecretRef != nil {
		if username, ok := credentials["username"]; ok {
			prom.username = string(username)
		} else {
			return nil, fmt.Errorf("%s credentials does not contain a username", provider.Type)
		}

		if password, ok := credentials["password"]; ok {
			prom.password = string(password)
		} else {
			return nil, fmt.Errorf("%s credentials does not contain a password", provider.Type)
		}
	}

	return &prom, nil
}

// RunQuery executes the promQL query and returns the the first result as float64
func (p *PrometheusProvider) RunQuery(query string) (float64, error) {
	query = url.QueryEscape(p.trimQuery(query))
	u, err := url.Parse(fmt.Sprintf("./api/v1/query?query=%s", query))
	if err != nil {
		return 0, fmt.Errorf("url.Parase failed: %w", err)
	}
	u.Path = path.Join(p.url.Path, u.Path)

	u = p.url.ResolveReference(u)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("http.NewRequest failed: %w", err)
	}

	if p.username != "" && p.password != "" {
		req.SetBasicAuth(p.username, p.password)
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()

	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading body: %w", err)
	}

	if 400 <= r.StatusCode {
		return 0, fmt.Errorf("error response: %s", string(b))
	}

	var result prometheusResponse
	err = json.Unmarshal(b, &result)
	if err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
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
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	return *value, nil
}

// IsOnline run simple Prometheus query and returns an error if the API is unreachable
func (p *PrometheusProvider) IsOnline() (bool, error) {
	value, err := p.RunQuery(prometheusOnlineQuery)
	if err != nil {
		return false, fmt.Errorf("running query failed: %w", err)
	}

	if value != float64(1) {
		return false, fmt.Errorf("value is not 1 for query: %s", prometheusOnlineQuery)
	}

	return true, nil
}

// trimQuery takes a promql query and removes whitespace
func (p *PrometheusProvider) trimQuery(query string) string {
	space := regexp.MustCompile(`\s+`)
	return space.ReplaceAllString(query, " ")
}
