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
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/signalfx/signalflow-client-go/signalflow"
	"github.com/signalfx/signalflow-client-go/signalflow/messages"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// https://docs.datadoghq.com/api/
const (
	signalFxMTSQueryPath   = "/v2/signalflow/execute"
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
		metricsQueryEndpoint:  strings.Replace(strings.Replace(address+signalFxMTSQueryPath, "http", "ws", 1), "api", "stream", 1),
		apiValidationEndpoint: strings.Replace(strings.Replace(address+signalFxValidationPath, "ws", "http", 1), "stream", "api", 1),
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

	sp.fromDelta = int64(signalFxFromDeltaMultiplierOnMetricInterval * md.Milliseconds())
	return &sp, nil
}

// RunQuery executes the query and converts the first result to float64
func (p *SplunkProvider) RunQuery(query string) (float64, error) {
	c, err := signalflow.NewClient(signalflow.StreamURL(p.metricsQueryEndpoint), signalflow.AccessToken(p.token))
	if err != nil {
		return 0, fmt.Errorf("error creating signalflow client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	now := time.Now().UnixMilli()
	comp, err := c.Execute(ctx, &signalflow.ExecuteRequest{
		Program:   query,
		Start:     time.Unix(0, (now-p.fromDelta)*time.Millisecond.Nanoseconds()),
		Stop:      time.Unix(0, now*time.Millisecond.Nanoseconds()),
		Immediate: true,
	})
	if err != nil {
		return 0, fmt.Errorf("error executing query: %w", err)
	}

	select {
	case dataMsg := <-comp.Data():
		payloads := slices.DeleteFunc(dataMsg.Payloads, func(msg messages.DataPayload) bool {
			return msg.Value() == nil
		})
		if len(payloads) < 1 {
			return 0, fmt.Errorf("invalid response: %w", ErrNoValuesFound)
		}
		_payloads := slices.Clone(payloads)
		slices.SortFunc(_payloads, func(i, j messages.DataPayload) int {
			return cmp.Compare(i.TSID, j.TSID)
		})
		if len(slices.CompactFunc(_payloads, func(i, j messages.DataPayload) bool { return i.TSID == j.TSID })) > 1 {
			return 0, fmt.Errorf("invalid response: %w", ErrMultipleValuesReturned)
		}
		return payloads[len(payloads)-1].Value().(float64), nil
	case <-time.After(p.timeout):
		err := comp.Stop(ctx)
		if err != nil {
			return 0, fmt.Errorf("error stopping query: %w", err)
		}
		return 0, fmt.Errorf("timeout waiting for query result")
	}
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
