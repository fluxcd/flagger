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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const (
	nerdGraphDefaultHost        = "https://api.newrelic.com/graphql"
	nerdGraphAPIKeySecretKey    = "newrelic_api_key"
	nerdGraphAccountIDSecretKey = "newrelic_account_id"
	nerdGraphAPIKeyHeaderKey    = "Api-Key"
	nerdGraphContentTypeHeader  = "application/json"

	nerdGraphRunQueryTemplate = `
	query ($query: Nrql!) {
	  actor {
	    account(id: %s) {
	      nrql(query: $query) {
	        results
	      }
	    }
	  }
	}`
)

// NerdGraphProvider executes New Relic NerdGraph queries
type NerdGraphProvider struct {
	endpoint  string
	timeout   time.Duration
	apiKey    string
	accountID string
	fromDelta int64
}

// nerdGraphQuery is the JSON payload for a GraphQL request
type nerdGraphQuery struct {
	Query string `json:"query"`
}

// nerdGraphGQLResponse is the generic wrapper for a GraphQL response
type nerdGraphGQLResponse struct {
	Data   map[string]any   `json:"data"`
	Errors []nerdGraphError `json:"errors"`
}

type nerdGraphPayload struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// nerdGraphError represents a single error in a GraphQL response
type nerdGraphError struct {
	Message string `json:"message"`
}

// NewNerdGraphProvider takes a canary spec, a provider spec and the credentials map, and
// returns a NewRelic client ready to execute queries against the NerdGraph API
func NewNerdGraphProvider(
	metricInterval string,
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte,
) (*NerdGraphProvider, error) {
	address := provider.Address
	if address == "" {
		address = nerdGraphDefaultHost
	}

	apiKey, ok := credentials[nerdGraphAPIKeySecretKey]
	if !ok {
		return nil, fmt.Errorf("newrelic credentials does not contain the key '%s'", nerdGraphAPIKeySecretKey)
	}

	accountIDBytes, ok := credentials[nerdGraphAccountIDSecretKey]
	if !ok {
		return nil, fmt.Errorf("newrelic credentials does not contain the key '%s'", nerdGraphAccountIDSecretKey)
	}
	accountID := string(accountIDBytes)
	if _, err := strconv.Atoi(accountID); err != nil {
		return nil, fmt.Errorf("newrelic account ID '%s' is not a valid integer: %w", accountID, err)
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}

	return &NerdGraphProvider{
		timeout:   5 * time.Second,
		endpoint:  address,
		apiKey:    string(apiKey),
		accountID: accountID,
		fromDelta: int64(md.Seconds()),
	}, nil
}

// RunQuery executes the NerdGraph query and returns the first numeric result
func (p *NerdGraphProvider) RunQuery(query string) (float64, error) {
	since := strconv.FormatInt(p.fromDelta, 10)
	fullQuery := fmt.Sprintf("%s SINCE %s SECONDS ago", query, since)
	queryTemplate := fmt.Sprintf(nerdGraphRunQueryTemplate, p.accountID)

	payload := nerdGraphPayload{
		Query: queryTemplate,
		Variables: map[string]any{
			"query": fullQuery,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("error marshalling payload: %w", err)
	}

	req, err := p.newNerdGraphRequest(payloadBytes)
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
		return 0, fmt.Errorf("error response: %s", string(b))
	}

	var res nerdGraphGQLResponse
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(res.Errors) > 0 {
		return 0, fmt.Errorf("nerdgraph query failed: %s", res.Errors[0].Message)
	}

	if res.Data == nil {
		return 0, fmt.Errorf("invalid response, no data found: %s", string(b))
	}

	// Recursively find the first numeric value in the first 'results' array
	val, err := findResultValue(res.Data)
	if err != nil {
		return 0, fmt.Errorf("error parsing nerdgraph response: %w, body: '%s'", err, string(b))
	}

	return val, nil
}

// IsOnline checks if the NerdGraph API is reachable and credentials are valid
func (p *NerdGraphProvider) IsOnline() (bool, error) {
	pingQuery := "{ actor { user { name } } }"
	query := nerdGraphQuery{Query: pingQuery}
	payload, err := json.Marshal(query)
	if err != nil {
		return false, fmt.Errorf("error marshaling ping query: %w", err)
	}

	req, err := p.newNerdGraphRequest(payload)
	if err != nil {
		return false, fmt.Errorf("error creating http.NewRequest: %w", err)
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

	var res nerdGraphGQLResponse
	if err := json.Unmarshal(b, &res); err != nil {
		return false, fmt.Errorf("error unmarshaling response: %w, '%s'", err, string(b))
	}

	if len(res.Errors) > 0 {
		return false, fmt.Errorf("nerdgraph query failed: %s", res.Errors[0].Message)
	}

	return true, nil
}

// newNerdGraphRequest creates a new HTTP POST request for the NerdGraph API
func (p *NerdGraphProvider) newNerdGraphRequest(payload []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", p.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("error creating http.NewRequest: %w", err)
	}

	req.Header.Set(nerdGraphAPIKeyHeaderKey, p.apiKey)
	req.Header.Set("Content-Type", nerdGraphContentTypeHeader)

	return req, nil
}

// findResultValue recursively searches a map for the first 'results' array,
// then returns the first float64 value from the first object in that array.
func findResultValue(data map[string]any) (float64, error) {
	for key, val := range data {
		// 1. Found a 'results' array?
		if key == "results" {
			if results, ok := val.([]any); ok && len(results) > 0 {
				// 2. Get the first object in the array
				if firstResult, ok := results[0].(map[string]any); ok {
					// 3. Find the first float64 value in that object
					for _, resultVal := range firstResult {
						if f, ok := resultVal.(float64); ok {
							return f, nil
						}
					}
				}
			}
		}

		// 4. Not 'results'? Recurse if it's another map
		if subMap, ok := val.(map[string]any); ok {
			if res, err := findResultValue(subMap); err == nil {
				return res, nil // Found it nested deeper
			}
		}
	}

	// 5. Searched everywhere, no result
	return 0, ErrNoValuesFound
}
