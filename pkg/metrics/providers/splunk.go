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

// https://docs.datadoghq.com/api/
const (
	signalFxMTSQueryPath   = "/v1/timeserieswindow"
	signalFxValidationPath = "/v2/metric?limit=1"

	signalFxTokenSecretKey = "sf_token_key"

	signalFxTokenHeaderKey = "X-SF-Token"

	signalFxFromDeltaMultiplierOnMetricInterval = 10
)

// SplunkProvider executes signalfx queries
type SplunkProvider struct {
	metricsQueryEndpoint  string
	apiValidationEndpoint string

	timeout   time.Duration
	token     string
	fromDelta int64
}

type splunkResponse struct {
	Data map[string][][]float64 `json:"data"`
}

// NewSplunkProvider takes a canary spec, a provider spec and the credentials map, and
// returns a Splunk client ready to execute queries against the API
func NewSplunkProvider(metricInterval string,
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*SplunkProvider, error) {

	address := provider.Address
	if address == "" {
		return nil, fmt.Errorf("splunk endpoint is not set")
	}

	sp := SplunkProvider{
		timeout:               5 * time.Second,
		metricsQueryEndpoint:  address + signalFxMTSQueryPath,
		apiValidationEndpoint: address + signalFxValidationPath,
	}

	if b, ok := credentials[signalFxTokenSecretKey]; ok {
		sp.token = string(b)
	} else {
		return nil, fmt.Errorf("splunk credentials does not contain sf_token_key")
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}

	sp.fromDelta = int64(signalFxFromDeltaMultiplierOnMetricInterval * md.Seconds())
	return &sp, nil
}

// RunQuery executes the query and converts the first result to float64
func (p *SplunkProvider) RunQuery(query string) (float64, error) {

	req, err := http.NewRequest("GET", p.metricsQueryEndpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Set(signalFxTokenHeaderKey, p.token)
	now := time.Now().Unix()
	q := req.URL.Query()
	q.Add("query", query)
	q.Add("startMS", strconv.FormatInt(now-p.fromDelta, 10))
	q.Add("endMS", strconv.FormatInt(now, 10))
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

	var res splunkResponse
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(res.Data) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	if len(res.Data) > 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrMultipleValuesReturned)
	}

	for _, v := range res.Data {
		vs := v[len(v)-1]
		if len(vs) < 1 {
			return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
		}
		return vs[1], nil
	}
	return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
}

// IsOnline calls the provider endpoint and returns an error if the API is unreachable
func (p *SplunkProvider) IsOnline() (bool, error) {
	req, err := http.NewRequest("GET", p.apiValidationEndpoint, nil)
	if err != nil {
		return false, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Add(signalFxTokenHeaderKey, p.token)

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
