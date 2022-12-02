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
	"fmt"
	"io"
	"net/http"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/machinebox/graphql"
)

const (
	healthCheckEndpoint = "/internal/l7check"
	queryEndpoint       = "/graphql"
)

type skywalkingResponse = map[string]struct {
	Label  string `json:"label"`
	Values struct {
		Values []struct {
			Value *float64 `json:"value"`
		} `json:"values"`
	} `json:"values"`
}

type duration struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Step  string `json:"step"`
}

// SkyWalkingProvider executes SkyWalking render URL API queries.
type SkyWalkingProvider struct {
	address  string
	timeout  time.Duration
	client   graphql.Client
	interval time.Duration
}

// NewSkyWalkingProvider takes a provider spec and credentials map,
// validates the address, extracts the  credentials map's username
// and password values if provided, and returns a skywalking client
// ready to execute queries against the skywalking render URL API.
func NewSkyWalkingProvider(metricInterval string, provider flaggerv1.MetricTemplateProvider, credentials map[string][]byte) (*SkyWalkingProvider, error) {
	md, err := time.ParseDuration(metricInterval)
	if err != nil {
		return nil, fmt.Errorf("error parsing metric interval: %w", err)
	}

	sw := SkyWalkingProvider{
		timeout:  5 * time.Second,
		address:  provider.Address,
		client:   *graphql.NewClient(provider.Address + queryEndpoint),
		interval: time.Duration(md.Nanoseconds()),
	}

	return &sw, nil
}

// RunQuery executes the skywalking render URL API query and returns the
// the first result as float64.
func (p *SkyWalkingProvider) RunQuery(query string) (float64, error) {
	req := graphql.NewRequest(query)
	req.Var("duration", duration{
		Start: time.Now().Add(-p.interval).Format("2006-01-02 1504"),
		End:   time.Now().Format("2006-01-02 1504"),
		Step:  "MINUTE",
	})
	res := skywalkingResponse{}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	err := p.client.Run(ctx, req, &res)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}

	for _, r := range res {
		for _, v := range r.Values.Values {
			if v.Value != nil {
				return *v.Value, nil
			}
		}
	}

	return 0, ErrNoValuesFound
}

// IsOnline runs a simple skywalking render URL API query and returns
// an error if the API is unreachable.
func (p *SkyWalkingProvider) IsOnline() (bool, error) {
	req, err := http.NewRequest("GET", p.address+healthCheckEndpoint, nil)
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

	// healthCheckEndpoint is added recently, for older versions of skywalking GET method is not allowed.
	if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusMethodNotAllowed {
		return false, fmt.Errorf("error response: %s", string(b))
	}

	return true, nil
}
