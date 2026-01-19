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
	"fmt"
	"net/url"
	"strings"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	externalmetrics_client "k8s.io/metrics/pkg/client/external_metrics"
)

// ExternalMetricsProvider fetches metrics from an ExternalMetricsProvider.
type ExternalMetricsProvider struct {
	timeout time.Duration
	client  externalmetrics_client.NamespacedMetricsGetter
}

// NewExternalMetricsProvider takes a canary spec, a provider spec, and
// returns a client ready to execute queries against the Service.
func NewExternalMetricsProvider(
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*ExternalMetricsProvider, error) {
	return newExternalMetricsProviderWithBuilder(
		provider, credentials, rest.InClusterConfig,
	)
}

// newExternalMetricsProviderWithBuilder is like NewExternalMetricsProvider but
// accepts a rest.Config builder function. Used for testing as InClusterConfig is hard to mock
func newExternalMetricsProviderWithBuilder(
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte,
	configBuilder func() (*rest.Config, error),
) (*ExternalMetricsProvider, error) {
	restConfig, err := configBuilder()
	if err != nil || restConfig == nil {
		return nil, fmt.Errorf("Not in a kubernetes cluster: %w", err)
	}

	// Handling overrides from MetricTemplateProvider
	if provider.Address != "" {
		restConfig.Host = provider.Address
	}
	restConfig.TLSClientConfig = rest.TLSClientConfig{
		Insecure: provider.InsecureSkipVerify,
	}
	if tokenBytes, ok := credentials["token"]; ok {
		restConfig.BearerToken = string(tokenBytes)
	}
	// TODO: handle user name/password auth if needed

	client, err := externalmetrics_client.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating external metric client: %w", err)
	}

	return &ExternalMetricsProvider{
		timeout: 5 * time.Second,
		client:  client,
	}, nil
}

// RunQuery retrieves the ExternalMetricValue from the External Metrics API
// at the ExternalMetricsProvider's Address, using the provided query string,
// and returns the *first* result as a float64.
func (p *ExternalMetricsProvider) RunQuery(query string) (float64, error) {
	// The Provider interface only allows a plain string query so decode it
	namespace, metricName, selector, err := parseExternalMetricsQuery(query)
	if err != nil {
		return 0, fmt.Errorf("error parsing metric query: %w", err)
	}

	nm := p.client.NamespacedMetrics(namespace)
	metricsList, err := nm.List(metricName, selector)
	if err != nil {
		return 0, fmt.Errorf("error querying external metrics API: %w", err)
	}

	if len(metricsList.Items) < 1 {
		return 0, fmt.Errorf("no external metrics found: %w", ErrNoValuesFound)
	}

	vs := metricsList.Items[0].Value.AsApproximateFloat64()

	return vs, nil
}

// IsOnline tests that the External Metrics API is reachable by looking for dummy metrics.
// If we don't get a network error, we assume the service is online.
func (p *ExternalMetricsProvider) IsOnline() (bool, error) {
	nm := p.client.NamespacedMetrics("kube-system")
	_, err := nm.List("dummy-metric", labels.Everything())

	if err != nil {
		return false, fmt.Errorf("external metrics service unavailable: %w", err)
	}
	return true, nil
}

// parseExternalMetricsQuery parses a query string in the format:
//   <namespace>/<metricName>?labelSelector=<urlencoded label selectors>
// where only the metricName is required.
// and returns the namespace, metricName, and labelSelector separately.
func parseExternalMetricsQuery(query string) (namespace string, metricName string, labelSelector labels.Selector, err error) {
	u, err := url.Parse("dummy:///" + query)
	if err != nil {
		return "", "", labels.Everything(), fmt.Errorf("malformed query string, expected <namespace>/<metricName>?labelSelector=<urlencoded label selectors>, got %s", query)
	}
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return "", "", labels.Everything(), fmt.Errorf("malformed query string, too many slashes, expected <namespace>/<metricName>?labelSelector=<urlencoded label selectors>, got %s", query)
	}

	namespace = "default"
	switch len(parts) {
	case 1:
		// Format: "metric"
		metricName = parts[0]
	case 2:
		// Format: "namespace/metric" or "/metric"
		if parts[0] != "" {
			namespace = parts[0]
		}
		metricName = parts[1]
	}
	if metricName == "" {
		return "", "", labels.Everything(), fmt.Errorf("metric name cannot be empty")
	}
	
	qp := u.Query()
	rawSelector := qp.Get("labelSelector")
	if rawSelector == "" {
		labelSelector = labels.Everything()
	} else {
		labelSelector, err = labels.Parse(rawSelector)
		if err != nil {
			return "", "", labels.Everything(), fmt.Errorf("error parsing label selector from string %s: %w", rawSelector, err)
		}
	}

	return namespace, metricName, labelSelector, nil
}
