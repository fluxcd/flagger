/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// https://www.dynatrace.com/support/help/dynatrace-api/environment-api/metric-v2/get-all-metrics/
const (
	dynatraceMetricsQueryPath = "/api/v2/metrics/query"
	dynatraceValidationPath   = "/api/v2/metrics?pageSize=1"

	dynatraceAPITokenSecretKey       = "dynatrace_token"
	dynatraceAuthorizationHeaderKey  = "Authorization"
	dynatraceAuthorizationHeaderType = "Api-Token"

	dynatraceDeltaMultiplierOnMetricInterval = 10
)

// DynatraceProvider executes dynatrace queries
type DynatraceProvider struct {
	metricsQueryEndpoint  string
	apiValidationEndpoint string

	timeout   time.Duration
	token     string
	fromDelta int64
}

type dynatraceResponse struct {
	Result []struct {
		Data []struct {
			Timestamps []int64   `json:"timestamps"`
			Values     []float64 `json:"values"`
		} `json:"data"`
	} `json:"result"`
}

// NewDynatraceProvider takes a canary spec, a provider spec and the credentials map, and
// returns a Dynatrace client ready to execute queries against the API
func NewDynatraceProvider(metricInterval string,
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*DynatraceProvider, error) {

	address := provider.Address
	if address == "" {
		return nil, fmt.Errorf("dynatrace endpoint is not set")
	}

	dt := DynatraceProvider{
		timeout:               5 * time.Second,
		metricsQueryEndpoint:  address + dynatraceMetricsQueryPath,
		apiValidationEndpoint: address + dynatraceValidationPath,
	}

	if b, ok := credentials[dynatraceAPITokenSecretKey]; ok {
		dt.token = string(b)
	} else {
		return nil, fmt.Errorf("dynatrace credentials does not contain dynatrace_token")
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}

	dt.fromDelta = int64(dynatraceDeltaMultiplierOnMetricInterval * md.Milliseconds())
	return &dt, nil
}

// RunQuery executes the dynatrace query against DynatraceProvider.metricsQueryEndpoint
// and returns the the first result as float64
func (p *DynatraceProvider) RunQuery(query string) (float64, error) {

	req, err := http.NewRequest("GET", p.metricsQueryEndpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Set(dynatraceAuthorizationHeaderKey, fmt.Sprintf("%s %s", dynatraceAuthorizationHeaderType, p.token))

	now := time.Now().Unix() * 1000
	q := req.URL.Query()
	q.Add("metricSelector", query)
	q.Add("resolution", "Inf")
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
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading body: %w", err)
	}

	if r.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("error response: %s: %w", string(b), err)
	}

	var res dynatraceResponse
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(res.Result) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	data := res.Result[0].Data
	if len(data) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	vs := data[len(data)-1]
	if len(vs.Values) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	return vs.Values[0], nil
}

// IsOnline calls the Dynatrace's metrics endpoint with token
// and returns an error if the endpoint fails
func (p *DynatraceProvider) IsOnline() (bool, error) {
	req, err := http.NewRequest("GET", p.apiValidationEndpoint, nil)
	if err != nil {
		return false, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Set(dynatraceAuthorizationHeaderKey, fmt.Sprintf("%s %s", dynatraceAuthorizationHeaderType, p.token))

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()
	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}

	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return false, fmt.Errorf("error reading body: %w", err)
	}

	if r.StatusCode != http.StatusOK {
		return false, fmt.Errorf("error response: %s", string(b))
	}

	return true, nil
}
