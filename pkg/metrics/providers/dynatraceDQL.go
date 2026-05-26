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
	"strings"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// https://developer.dynatrace.com/develop/platform-services/services/grail-service/
const (
	dynatraceDQLAPIPath = "/platform/storage/query/v1"

	dynatraceDQLAPITokenSecretKey       = "dynatrace_token"
	dynatraceDQLAuthorizationHeaderKey  = "Authorization"
	dynatraceDQLAuthorizationHeaderType = "Bearer"

	//dynatraceDeltaMultiplierOnMetricInterval = 10
)

// DynatraceDQLProvider executes dynatrace queries
type DynatraceDQLProvider struct {
	apiRoot string

	timeout   time.Duration
	token     string
	fromDelta time.Duration
}

// NewDynatraceDQLProvider takes a canary spec, a provider spec and the credentials map, and
// returns a Dynatrace client ready to execute queries against the API
func NewDynatraceDQLProvider(metricInterval string,
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*DynatraceDQLProvider, error) {

	address := provider.Address
	if address == "" {
		return nil, fmt.Errorf("dynatrace endpoint is not set")
	} else if strings.HasSuffix(address, "/") {
		address = address[:len(address)-1]
	}

	dt := DynatraceDQLProvider{
		timeout: 5 * time.Second,
		apiRoot: address + dynatraceDQLAPIPath,
	}

	if b, ok := credentials[dynatraceDQLAPITokenSecretKey]; ok {
		dt.token = string(b)
	} else {
		return nil, fmt.Errorf("dynatrace credentials does not contain dynatrace_token")
	}

	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}
	if md > 0 {
		dt.fromDelta = md * -1
	} else {
		dt.fromDelta = md
	}

	return &dt, nil
}

func (p *DynatraceDQLProvider) _queryPoll(requestToken string) (*QueryPollResponse, []byte, error) {
	url := p.apiRoot + "/query:poll"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error http.NewRequest: %w", err)
	}

	q := req.URL.Query()
	q.Add("request-token", requestToken)
	req.URL.RawQuery = q.Encode()

	b, err := p._doRequest(req)
	if err != nil {
		return nil, nil, err
	}

	var res QueryPollResponse
	err = json.Unmarshal(b, &res)
	if err != nil {
		return nil, b, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	return &res, b, nil
}
func (p *DynatraceDQLProvider) _queryExecute(query ExecuteRequest) (*QueryStartResponse, []byte, error) {
	url := p.apiRoot + "/query:execute"

	marshalled, err := json.Marshal(query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request json: %w", err)
	}

	b, err := p._doJSONPost(marshalled, url)
	if err != nil {
		return nil, nil, err
	}

	var res QueryStartResponse
	err = json.Unmarshal(b, &res)
	if err != nil {
		return nil, b, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	return &res, b, nil
}
func (p *DynatraceDQLProvider) _queryVerify(query VerifyRequest) (*VerifyResponse, []byte, error) {
	url := p.apiRoot + "/query:verify"

	marshalled, err := json.Marshal(query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request json: %w", err)
	}

	b, err := p._doJSONPost(marshalled, url)
	if err != nil {
		return nil, nil, err
	}

	var res VerifyResponse
	err = json.Unmarshal(b, &res)
	if err != nil {
		return nil, b, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	return &res, b, nil
}
func (p *DynatraceDQLProvider) _doJSONPost(body []byte, url string) ([]byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error http.NewRequest: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return p._doRequest(req)
}

func (p *DynatraceDQLProvider) _doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set(dynatraceDQLAuthorizationHeaderKey, fmt.Sprintf("%s %s", dynatraceDQLAuthorizationHeaderType, p.token))
	req.Header.Set("Accept", "application/json")

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()
	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %w", err)
	}

	if r.StatusCode < 200 || 300 <= r.StatusCode {
		return nil, fmt.Errorf("bad status code %d: body: %s: %w", r.StatusCode, string(b), err)
	}
	return b, nil
}

// RunQuery executes the dynatrace query against DynatraceDQLProvider.dynatraceDQLAPIPath/query:execute and query:poll
// and returns the the first result as float64
func (p *DynatraceDQLProvider) RunQuery(query string) (float64, error) {
	// First call query:execute to start the query
	// Then call query:poll till it returns a result
	// guaranteed to be under a minute

	now := time.Now()
	start := now.Add(p.fromDelta)

	tz := "UTC"
	nowStr := now.Format(time.RFC3339)
	fromStr := start.Format(time.RFC3339)
	executeRequest := ExecuteRequest{
		Query:                 query,
		Timezone:              &tz,
		DefaultTimeframeEnd:   &nowStr,
		DefaultTimeframeStart: &fromStr,
	}

	res, b, err := p._queryExecute(executeRequest)
	if err != nil {
		return 0, fmt.Errorf("error posting query:execute: %w", err)
	}

	var result *QueryResult
	switch res.State {
	case CANCELLED:
		fallthrough
	case RESULTGONE:
		fallthrough
	case FAILED:
		return 0, fmt.Errorf("query:execute failed, in state: %s: %s", res.State, string(b))
	case NOTSTARTED:
		fallthrough
	case RUNNING:
		for result == nil {
			pollRes, pollB, err := p._queryPoll(*res.RequestToken)
			if err != nil {
				return 0, fmt.Errorf("error getting query:poll: %w", err)
			} else {
				switch pollRes.State {
				case CANCELLED:
					fallthrough
				case RESULTGONE:
					fallthrough
				case FAILED:
					return 0, fmt.Errorf("query:poll failed, in state: %s: %s", pollRes.State, string(b))
				case SUCCEEDED:
					result = pollRes.Result
					b = pollB
				case NOTSTARTED:
					fallthrough
				case RUNNING:
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	case SUCCEEDED:
		result = res.Result
	}

	if len(result.Records) < 1 {
		return 0, fmt.Errorf("invalid response: no results: %s: %w", string(b), ErrNoValuesFound)
	}

	record := result.Records[len(result.Records)-1]
	val, ok := (*record)["r"]
	if !ok {
		return 0, fmt.Errorf("invalid response data doesn't contain 'r' property: %s: %w", string(b), ErrNoValuesFound)
	}

	var ret float64
	err = json.Unmarshal(*val, &ret)
	if err != nil {
		return 0, fmt.Errorf("error unmarshaling final data value into float64: %w, '%s'", err, string(*val))
	}

	return ret, nil
}

// IsOnline calls DynatraceDQLProvider.dynatraceDQLAPIPath/query:verify with
// token and returns an error if the endpoint fails
func (p *DynatraceDQLProvider) IsOnline() (bool, error) {
	query := VerifyRequest{
		Query: `timeseries{cpu=avg(dt.host.cpu.usage),filter:matchesValue(dt.smartscape.host,"HOST-001109335619D5DD")},from:now()-5m|fields r=arraySum(cpu)`,
	}

	res, b, err := p._queryVerify(query)
	if err != nil {
		return false, fmt.Errorf("error posting query:verify: %w", err)
	}

	if !res.Valid {
		return false, fmt.Errorf("query:verify says our valid query is invalid: %s", string(b))
	}

	return true, nil
}
