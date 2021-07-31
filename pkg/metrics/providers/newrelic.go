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
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const (
	newrelicInsightsDefaultHost = "https://insights-api.newrelic.com"

	newrelicQueryKeySecretKey  = "newrelic_query_key"
	newrelicAccountIdSecretKey = "newrelic_account_id"

	newrelicQueryKeyHeaderKey = "X-Query-Key"
)

// NewRelicProvider executes newrelic queries
type NewRelicProvider struct {
	insightsQueryEndpoint string

	timeout   time.Duration
	queryKey  string
	fromDelta int64
}

type newRelicResponse struct {
	Results []struct {
		Result *float64 `json:"result"`
	} `json:"results"`
}

// NewNewRelicProvider takes a canary spec, a provider spec and the credentials map, and
// returns a NewRelic client ready to execute queries against the Insights API
func NewNewRelicProvider(
	metricInterval string,
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte,
) (*NewRelicProvider, error) {
	address := provider.Address
	if address == "" {
		address = newrelicInsightsDefaultHost
	}

	accountId, ok := credentials[newrelicAccountIdSecretKey]
	if !ok {
		return nil, fmt.Errorf("newrelic credentials does not contain the key '%s'", newrelicAccountIdSecretKey)
	}

	queryEndpoint := fmt.Sprintf("%s/v1/accounts/%s/query", address, accountId)
	nr := NewRelicProvider{
		timeout:               5 * time.Second,
		insightsQueryEndpoint: queryEndpoint,
	}

	if b, ok := credentials[newrelicQueryKeySecretKey]; ok {
		nr.queryKey = string(b)
	} else {
		return nil, fmt.Errorf("newrelic credentials does not contain the key ''%s", newrelicQueryKeySecretKey)
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}

	nr.fromDelta = int64(md.Seconds())
	return &nr, nil
}

// RunQuery executes the new relic query against the New Relic Insights API
// and returns the the first result
func (p *NewRelicProvider) RunQuery(query string) (float64, error) {
	req, err := p.newInsightsRequest(query)
	if err != nil {
		return 0, err
	}

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

	var res newRelicResponse
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(res.Results) != 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	if res.Results[0].Result == nil {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	return *res.Results[0].Result, nil
}

// IsOnline calls the NewRelic's insights API with
// and returns an error if the request is rejected
func (p *NewRelicProvider) IsOnline() (bool, error) {
	req, err := p.newInsightsRequest("SELECT * FROM Metric")
	if err != nil {
		return false, fmt.Errorf("error http.NewRequest: %w", err)
	}

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

func (p *NewRelicProvider) newInsightsRequest(query string) (*http.Request, error) {
	req, err := http.NewRequest("GET", p.insightsQueryEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Set(newrelicQueryKeyHeaderKey, p.queryKey)

	q := req.URL.Query()
	q.Add("nrql", fmt.Sprintf("%s SINCE %d seconds ago", query, p.fromDelta))
	req.URL.RawQuery = q.Encode()

	return req, nil
}
