/*
Copyright 2025 The Flux authors

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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const (
	signozTokenSecretKey                      = "apiKey"
	signozHeaderKey                           = "SIGNOZ-API-KEY"
	signozFromDeltaMultiplierOnMetricInterval = 10
)

var SignozAPIPath = "/api/v5/query_range"

type SignozProvider struct {
	timeout   time.Duration
	url       url.URL
	headers   http.Header
	apiKey    string
	client    *http.Client
	queryPath string
	fromDelta int64
}

type signozResponse struct {
	Status string `json:"status"`
	Data   struct {
		Type string `json:"type"`
		Data struct {
			Results []struct {
				QueryName    string `json:"queryName"`
				Aggregations []struct {
					Index  int    `json:"index"`
					Alias  string `json:"alias"`
					Series []struct {
						Labels []struct {
							Key struct {
								Name string `json:"name"`
							} `json:"key"`
							Value string `json:"value"`
						} `json:"labels"`
						Values []struct {
							Timestamp int64   `json:"timestamp"`
							Value     float64 `json:"value"`
							Partial   bool    `json:"partial,omitempty"`
						} `json:"values"`
					} `json:"series"`
				} `json:"aggregations"`
			} `json:"results"`
		} `json:"data"`
	} `json:"data"`
}

// NewSignozProvider takes a provider spec and the credentials map,
// validates the address, extracts the API key from the provided Secret,
// and returns a client ready to execute requests against the SigNoz API.
func NewSignozProvider(metricInterval string, provider flaggerv1.MetricTemplateProvider, credentials map[string][]byte) (*SignozProvider, error) {
	signozURL, err := url.Parse(provider.Address)
	if provider.Address == "" || err != nil {
		return nil, fmt.Errorf("%s address %s is not a valid URL", provider.Type, provider.Address)
	}

	sp := SignozProvider{
		timeout:   30 * time.Second,
		url:       *signozURL,
		headers:   provider.Headers,
		client:    http.DefaultClient,
		queryPath: SignozAPIPath,
	}

	if provider.InsecureSkipVerify {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		sp.client = &http.Client{Transport: t}
	}

	if provider.SecretRef != nil {
		if apiKey, ok := credentials[signozTokenSecretKey]; ok {
			sp.apiKey = string(apiKey)
		} else {
			return nil, fmt.Errorf("%s credentials does not contain %s", provider.Type, signozTokenSecretKey)
		}
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}
	sp.fromDelta = int64(signozFromDeltaMultiplierOnMetricInterval * md.Milliseconds())

	return &sp, nil
}

// RunQuery posts the provided JSON payload to SigNoz query_range and
// returns a single float64 value derived from the response.
//
// Expectations:
// - The input `query` is a valid JSON document per SigNoz Query Range API.
// - The response must contain a single time series (or single value).
// - Returns ErrMultipleValuesReturned when multiple series are found.
// - Returns ErrNoValuesFound on missing/NaN values.
func (p *SignozProvider) RunQuery(query string) (float64, error) {
	now := time.Now().UnixMilli()
	start := now - p.fromDelta
	var q map[string]interface{}
	if err := json.Unmarshal([]byte(query), &q); err != nil {
		return 0, fmt.Errorf("error unmarshaling query: %w", err)
	}
	q["start"] = start
	q["end"] = now
	q["requestType"] = "time_series"

	payload, err := json.Marshal(q)
	if err != nil {
		return 0, fmt.Errorf("error marshaling updated query: %w", err)
	}

	// Build URL
	u, err := url.Parse("." + p.queryPath)
	if err != nil {
		return 0, fmt.Errorf("url.Parse failed: %w", err)
	}
	u.Path = path.Join(p.url.Path, u.Path)
	u = p.url.ResolveReference(u)

	req, err := http.NewRequest("POST", u.String(), io.NopCloser(strings.NewReader(string(payload))))
	if err != nil {
		return 0, fmt.Errorf("http.NewRequest failed: %w", err)
	}
	if p.headers != nil {
		req.Header = p.headers.Clone()
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set(signozHeaderKey, p.apiKey)
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()
	resp, err := p.client.Do(req.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("error response: %s", string(body))
	}

	var sr signozResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(body))
	}

	if sr.Status != "success" {
		return 0, fmt.Errorf("SigNoz query failed with status: %s", sr.Status)
	}

	if len(sr.Data.Data.Results) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	// For formula queries, we expect a single result with aggregations
	result := sr.Data.Data.Results[0]
	if len(result.Aggregations) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	// Get the first aggregation (usually the formula result)
	agg := result.Aggregations[0]
	if len(agg.Series) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	// For formula results, there should be a single series
	if len(agg.Series) > 1 {
		return 0, fmt.Errorf("%w", ErrMultipleValuesReturned)
	}

	series := agg.Series[0]
	if len(series.Values) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	// Get the last non-partial value from the series
	var lastValue float64
	found := false
	for i := len(series.Values) - 1; i >= 0; i-- {
		if !series.Values[i].Partial {
			lastValue = series.Values[i].Value
			found = true
			break
		}
	}

	if !found {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	if math.IsNaN(lastValue) {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	return lastValue, nil
}

// IsOnline probes SigNoz health.
func (p *SignozProvider) IsOnline() (bool, error) {
	healthURL := p.url
	healthURL.Path = path.Join(p.url.Path, "/api/v1/health")

	req, err := http.NewRequest("GET", healthURL.String(), nil)
	if err != nil {
		return false, fmt.Errorf("http.NewRequest failed: %w", err)
	}
	if p.headers != nil {
		req.Header = p.headers.Clone()
	}
	if p.apiKey != "" {
		req.Header.Set(signozHeaderKey, p.apiKey)
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()

	resp, err := p.client.Do(req.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("health check failed with status %d: %s", resp.StatusCode, string(b))
	}
	return true, nil
}
