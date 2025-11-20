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
	"net"
	"net/http"
	"net/url"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

// ExternalMetricsProvider executes datadog queries
type ExternalMetricsProvider struct {
	metricServiceEndpoint string

	timeout   time.Duration
	unsafeSsl bool
}

// NewExternalMetricsProvider takes a canary spec, a provider spec, and
// returns a client ready to execute queries against the Service
func NewExternalMetricsProvider(metricInterval string,
	provider flaggerv1.MetricTemplateProvider) (*ExternalMetricsProvider, error) {

	if provider.Address == "" {
		return nil, fmt.Errorf("the Url of the external metric service must be provided")
	}

	externalMetrics := ExternalMetricsProvider{
		metricServiceEndpoint: fmt.Sprintf("%s/apis/external.metrics.k8s.io/v1beta1", provider.Address),
		timeout:               5 * time.Second,
		unsafeSsl:             provider.InsecureSkipVerify,
	}

	return &externalMetrics, nil
}

// RunQuery retrieves the ExternalMetricValue from the ExternalMetricsProvider.metricServiceUrl
// and returns the first result as a float64
func (p *ExternalMetricsProvider) RunQuery(query string) (float64, error) {

	metricsQueryUrl := fmt.Sprintf("%s/namespaces/%s", p.metricServiceEndpoint, query)
	//TODO add labelSelector as queryString (in the docs of this provider as it's embedded in the query string)

	req, err := http.NewRequest("GET", metricsQueryUrl, nil)
	if err != nil {
		return 0, fmt.Errorf("error http.NewRequest: %w", err)
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

	var res external_metrics.ExternalMetricValueList
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(res.Items) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(b), ErrNoValuesFound)
	}

	// TODO
	vs := res.Items[0].Value.AsApproximateFloat64()

	return vs, nil
}

// IsOnline will only check the TCP endpoint reachability,
// given that external metric servers don't have a common health check endpoint defined
func (p *ExternalMetricsProvider) IsOnline() (bool, error) {
	var d net.Dialer

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	metricServiceUrl, err := url.Parse(p.metricServiceEndpoint)
	if err != nil {
		return false, fmt.Errorf("error parsing metric service url: %w", err)
	}

	conn, err := d.DialContext(ctx, "tcp", metricServiceUrl.Host)
	defer conn.Close()
	if err != nil {
		return false, fmt.Errorf("connection failed: %w", err)
	}
	return true, err
}
