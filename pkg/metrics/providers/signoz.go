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
	"strconv"
	"strings"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// SignozAPIPath is the default query range endpoint appended to the base address.
var SignozAPIPath = "/api/v5/query_range"

// SignozProvider executes SigNoz Query Range API requests
type SignozProvider struct {
	timeout   time.Duration
	url       url.URL
	headers   http.Header
	apiKey    string
	client    *http.Client
	queryPath string
}

// signozResponse models the SigNoz Query Range API response structure
type signozResponse struct {
	Data struct {
		Result []struct {
			Series []struct {
				Labels      map[string]string `json:"labels"`
				LabelString string            `json:"labelString"`
				Values      []struct {
					Timestamp int64  `json:"timestamp"`
					Value     string `json:"value"`
				} `json:"values"`
			} `json:"series"`
			QueryName string `json:"queryName"`
		} `json:"result"`
	} `json:"data"`
}

// NewSignozProvider takes a provider spec and the credentials map,
// validates the address, extracts the API key from the provided Secret,
// and returns a client ready to execute requests against the SigNoz API.
func NewSignozProvider(provider flaggerv1.MetricTemplateProvider, credentials map[string][]byte) (*SignozProvider, error) {
	signozURL, err := url.Parse(provider.Address)
	if provider.Address == "" || err != nil {
		return nil, fmt.Errorf("%s address %s is not a valid URL", provider.Type, provider.Address)
	}

	sp := SignozProvider{
		timeout:   5 * time.Second,
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
		if apiKey, ok := credentials["apiKey"]; ok {
			sp.apiKey = string(apiKey)
		} else {
			return nil, fmt.Errorf("%s credentials does not contain %s", provider.Type, "apiKey")
		}
	}

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
	u, err := url.Parse("." + p.queryPath)
	if err != nil {
		return 0, fmt.Errorf("url.Parse failed: %w", err)
	}
	u.Path = path.Join(p.url.Path, u.Path)
	u = p.url.ResolveReference(u)

	req, err := http.NewRequest("POST", u.String(), io.NopCloser(strings.NewReader(query)))
	if err != nil {
		return 0, fmt.Errorf("http.NewRequest failed: %w", err)
	}

	if p.headers != nil {
		req.Header = p.headers
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("SIGNOZ-API-KEY", p.apiKey)
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()

	r, err := p.client.Do(req.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading body: %w", err)
	}

	if r.StatusCode >= 400 {
		return 0, fmt.Errorf("error response: %s", string(b))
	}

	var resp signozResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	// Ensure we have results
	if len(resp.Data.Result) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}
	if len(resp.Data.Result) > 1 {
		return 0, fmt.Errorf("%w", ErrMultipleValuesReturned)
	}

	result := resp.Data.Result[0]

	// Check for multiple series
	if len(result.Series) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}
	if len(result.Series) > 1 {
		return 0, fmt.Errorf("%w", ErrMultipleValuesReturned)
	}

	series := result.Series[0]
	if len(series.Values) == 0 {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	// Get the last value from the series
	lastValue := series.Values[len(series.Values)-1]
	f, err := strconv.ParseFloat(lastValue.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing value: %w", err)
	}

	if math.IsNaN(f) {
		return 0, fmt.Errorf("%w", ErrNoValuesFound)
	}

	return f, nil
}

// IsOnline runs a minimal query and expects a value of 1
func (p *SignozProvider) IsOnline() (bool, error) {
	now := time.Now().UnixMilli()
	body := fmt.Sprintf(`{"start": %d, "end": %d, "requestType": "time_series", "compositeQuery": {"queries": [{"type": "builder_formula", "spec": {"name": "F1", "expression": "1", "disabled": false}}]}}`, now-60000, now)

	v, err := p.RunQuery(body)
	if err != nil {
		return false, fmt.Errorf("running query failed: %w", err)
	}
	if v != float64(1) {
		return false, fmt.Errorf("value is not 1 for query: builder_formula 1")
	}
	return true, nil
}
