package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// https://docs.datadoghq.com/api/
const (
	datadogDefaultHost = "https://api.datadoghq.com"

	datadogMetricsQueryPath     = "/api/v1/query"
	datadogAPIKeyValidationPath = "/api/v1/validate"

	datadogAPIKeySecretKey = "datadog_api_key"
	datadogAPIKeyHeaderKey = "DD-API-KEY"

	datadogApplicationKeySecretKey = "datadog_application_key"
	datadogApplicationKeyHeaderKey = "DD-APPLICATION-KEY"

	datadogFromDeltaMultiplierOnMetricInterval = 10
)

// DatadogProvider executes datadog queries
type DatadogProvider struct {
	metricsQueryEndpoint     string
	apiKeyValidationEndpoint string

	timeout        time.Duration
	apiKey         string
	applicationKey string
	fromDelta      int64
}

type datadogResponse struct {
	Series []struct {
		Pointlist [][]float64 `json:"pointlist"`
	}
}

// NewDatadogProvider takes a canary spec, a provider spec and the credentials map, and
// returns a Datadog client ready to execute queries against the API
func NewDatadogProvider(metricInterval string,
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*DatadogProvider, error) {

	address := provider.Address
	if address == "" {
		address = datadogDefaultHost
	}

	dd := DatadogProvider{
		timeout:                  5 * time.Second,
		metricsQueryEndpoint:     address + datadogMetricsQueryPath,
		apiKeyValidationEndpoint: address + datadogAPIKeyValidationPath,
	}

	if b, ok := credentials[datadogAPIKeySecretKey]; ok {
		dd.apiKey = string(b)
	} else {
		return nil, fmt.Errorf("datadog credentials does not contain datadog_api_key")
	}

	if b, ok := credentials[datadogApplicationKeySecretKey]; ok {
		dd.applicationKey = string(b)
	} else {
		return nil, fmt.Errorf("datadog credentials does not contain datadog_application_key")
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}

	dd.fromDelta = int64(datadogFromDeltaMultiplierOnMetricInterval * md.Seconds())
	return &dd, nil
}

// RunQuery executes the datadog query against DatadogProvider.metricsQueryEndpoint
// and returns the the first result as float64
func (p *DatadogProvider) RunQuery(query string) (float64, error) {

	req, err := http.NewRequest("GET", p.metricsQueryEndpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Set(datadogAPIKeyHeaderKey, p.apiKey)
	req.Header.Set(datadogApplicationKeyHeaderKey, p.applicationKey)
	now := time.Now().Unix()
	q := req.URL.Query()
	q.Add("query", query)
	q.Add("from", strconv.FormatInt(now-p.fromDelta, 10))
	q.Add("to", strconv.FormatInt(now, 10))
	req.URL.RawQuery = q.Encode()

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

	if r.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("error response: %s: %w", string(b), err)
	}

	var res datadogResponse
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(res.Series) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	pl := res.Series[0].Pointlist
	if len(pl) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	vs := pl[len(pl)-1]
	if len(vs) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	return vs[1], nil
}

// IsOnline calls the Datadog's validation endpoint with api keys
// and returns an error if the validation fails
func (p *DatadogProvider) IsOnline() (bool, error) {
	req, err := http.NewRequest("GET", p.apiKeyValidationEndpoint, nil)
	if err != nil {
		return false, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Add(datadogAPIKeyHeaderKey, p.apiKey)
	req.Header.Add(datadogApplicationKeyHeaderKey, p.applicationKey)

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()
	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}

	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return false, fmt.Errorf("error reading body: %w", err)
	}

	if r.StatusCode != http.StatusOK {
		return false, fmt.Errorf("error response: %s", string(b))
	}

	return true, nil
}
