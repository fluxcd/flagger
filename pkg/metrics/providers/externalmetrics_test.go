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
	json2 "encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	testMetricValue         = 1.11111
	testMetricName          = "myMetric"
	testMetricNamespace     = "default"
	testBearerToken         = "mytoken"
	testMetricServerAddress = "https://external-metrics.default.svc.cluster.local"
)

var (
	testMetricLabels       = [...]string{"label1"}
	testMetricLabelsValues = [...]string{"value1"}
)

func TestNewExternalMetricsProvider(t *testing.T) {
	cred := map[string][]byte{
		applicationBearerToken: []byte(testBearerToken),
	}

	providermetric:= flaggerv1.MetricTemplateProvider{
		Address:            testMetricServerAddress,
		InsecureSkipVerify: false,
	}

	// Should be OK
	emp, err := NewExternalMetricsProvider("100s", provider, cred)
	require.NoError(t, err)

	assert.Equal(t, fmt.Sprintf("%s/apis/external.metrics.k8s.io/v1beta1", testMetricServerAddress), emp.metricServiceEndpoint)
	assert.Equal(t, 5*time.Second, emp.timeout)
	assert.Equal(t, testBearerToken, emp.bearerToken)

	// No token and none in the filesystem
	_, err = NewExternalMetricsProvider("100s", provider, map[string][]byte{})
	require.Error(t, err)
}

func TestExternalMetrics_RunQuery(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		eq := fmt.Sprintf("%s/%s?%s=%s", testMetricNamespace, testMetricName, testMetricLabels[0], testMetricLabelsValues[0])

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			aq := fmt.Sprintf("%s?%s", r.URL.EscapedPath(), r.URL.RawQuery)
			assert.Equal(t, fmt.Sprintf("%s%s%s", metricServiceEndpointPath, namespacesPath, eq), aq)
			assert.Equal(t, fmt.Sprintf("Bearer %s", testBearerToken), r.Header.Get(autorisationHeaderKey))

			q, err := resource.ParseQuantity(strconv.FormatFloat(testMetricValue, 'f', -1, 64))
			assert.NoError(t, err)

			ret := &external_metrics.ExternalMetricValueList{
				TypeMeta: v1.TypeMeta{
					APIVersion: "external.metrics.k8s.io/v1beta1",
					Kind:       "ExternalMetricValueList",
				},
				ListMeta: v1.ListMeta{},
				Items: []external_metrics.ExternalMetricValue{
					{
						MetricName: testMetricName,
						MetricLabels: map[string]string{
							testMetricLabels[0]: testMetricLabelsValues[0],
						},
						Value: q,
					},
				},
			}
			json, err := json2.Marshal(ret)
			assert.NoError(t, err)
			w.Write(json)
		}))
		defer ts.Close()

		dp, err := NewExternalMetricsProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				applicationBearerToken: []byte(testBearerToken),
			},
		)
		require.NoError(t, err)

		f, err := dp.RunQuery(eq)
		require.NoError(t, err)
		assert.Equal(t, testMetricValue, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ret := &external_metrics.ExternalMetricValueList{
				TypeMeta: v1.TypeMeta{
					APIVersion: "external.metrics.k8s.io/v1beta1",
					Kind:       "ExternalMetricValueList",
				},
				ListMeta: v1.ListMeta{},
				Items:    []external_metrics.ExternalMetricValue{},
			}
			json, err := json2.Marshal(ret)
			assert.NoError(t, err)
			w.Write(json)
		}))
		defer ts.Close()

		dp, err := NewExternalMetricsProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{},
		)
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
	t.Run("TLS verify", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := resource.MustParse("1")
			ret := &external_metrics.ExternalMetricValueList{
				TypeMeta: v1.TypeMeta{
					APIVersion: "external.metrics.k8s.io/v1beta1",
					Kind:       "ExternalMetricValueList",
				},
				ListMeta: v1.ListMeta{},
				Items: []external_metrics.ExternalMetricValue{
					{
						MetricName: testMetricName,
						MetricLabels: map[string]string{
							testMetricLabels[0]: testMetricLabelsValues[0],
						},
						Value: v,
					},
				},
			}
			json, err := json2.Marshal(ret)
			assert.NoError(t, err)
			w.Write(json)
		}))
		defer ts.Close()

		// verify that there's an error without InsecureSkipVerify
		dp, err := NewExternalMetricsProvider("1m",
			flaggerv1.MetricTemplateProvider{
				Address:            ts.URL,
				InsecureSkipVerify: false,
			},
			map[string][]byte{},
		)
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.Error(t, err)

		// Now verify that there's no error when skipping TLS verification
		dp, err = NewExternalMetricsProvider("1m",
			flaggerv1.MetricTemplateProvider{
				Address:            ts.URL,
				InsecureSkipVerify: true,
			},
			map[string][]byte{},
		)
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.NoError(t, err)
	})
}
