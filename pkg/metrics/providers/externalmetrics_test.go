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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/inf.v0"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stesting "k8s.io/client-go/testing"
	emv1beta1 "k8s.io/metrics/pkg/apis/external_metrics/v1beta1"
	fakeemc "k8s.io/metrics/pkg/client/external_metrics/fake"
)

const (
	testMetricName          = "myMetric"
	testMetricNamespace     = "default"
	testMetricServerAddress = "https://external-metrics.default.svc.cluster.local"
	testQuery               = "default/myMetric?labelSelector=label1%3Dvalue1"
)

var (
	testMetricLabels       = [...]string{"label1"}
	testMetricLabelsValues = [...]string{"value1"}
	// 11111e-4 = 1.1111
	testMetricValue = resource.NewDecimalQuantity(*inf.NewDec(11111, 4), resource.DecimalSI)
)

func TestExternalMetrics_NewProvider(t *testing.T) {
	tests := []struct {
		name               string
		Address            string
		InsecureSkipVerify bool
		creds              map[string][]byte
		config             *rest.Config
	}{
		{
			name:               "Custom provider address and token",
			Address:            testMetricServerAddress,
			InsecureSkipVerify: false,
			creds: map[string][]byte{
				"token": []byte("test-token"),
			},
			config: &rest.Config{},
		},
		{
			name:               "In cluster, automatic address and token",
			Address:            "",
			InsecureSkipVerify: true,
			creds:              map[string][]byte{},
			config: &rest.Config{
				Host:            "https://kubernetes.default.svc",
				BearerToken:     "fake-token",
				TLSClientConfig: rest.TLSClientConfig{Insecure: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mtp := flaggerv1.MetricTemplateProvider{
				Address:            tt.Address,
				InsecureSkipVerify: tt.InsecureSkipVerify,
			}
			emp, err := NewExternalMetricsProvider(mtp, tt.creds, tt.config)
			require.NoError(t, err)
			require.NotNil(t, emp)
		})
	}
}

func TestExternalMetrics_NewProvider_NilConfig(t *testing.T) {
	mtp := flaggerv1.MetricTemplateProvider{}
	_, err := NewExternalMetricsProvider(mtp, nil, nil)
	require.Error(t, err)
}

func TestExternalMetrics_ParseQuery(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		wantNamespace     string
		wantMetricName    string
		wantLabelSelector string
		wantErr           bool
	}{
		{
			name:              "General case",
			query:             testQuery,
			wantNamespace:     testMetricNamespace,
			wantMetricName:    testMetricName,
			wantLabelSelector: labels.Set{testMetricLabels[0]: testMetricLabelsValues[0]}.AsSelector().String(),
			wantErr:           false,
		},
		{
			name:              "Still OK without labelSelector",
			query:             testQuery[:strings.Index(testQuery, "?")],
			wantNamespace:     testMetricNamespace,
			wantMetricName:    testMetricName,
			wantLabelSelector: labels.Everything().String(),
			wantErr:           false,
		},
		{
			name:              "No namespace uses default",
			query:             "/metric_only",
			wantNamespace:     "default",
			wantMetricName:    "metric_only",
			wantLabelSelector: labels.Everything().String(),
			wantErr:           false,
		},
		{
			name:    "Missing metric name - namespaceonly",
			query:   "namespaceonly/",
			wantErr: true,
		},
		{
			name:    "Missing metric name - slash only",
			query:   "/",
			wantErr: true,
		},
		{
			name:    "Missing metric name - empty",
			query:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNamespace, gotMetricName, gotLabelSelector, err := parseExternalMetricsQuery(tt.query)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantNamespace, gotNamespace)
				assert.Equal(t, tt.wantMetricName, gotMetricName)
				assert.Equal(t, tt.wantLabelSelector, gotLabelSelector.String())
			}
		})
	}
}

func TestExternalMetrics_RunQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		setup   func(*fakeemc.FakeExternalMetricsClient)
		want    float64
		wantErr bool
	}{
		{
			name:  "Full query with label selector",
			query: testQuery,
			setup: func(client *fakeemc.FakeExternalMetricsClient) {
				client.Fake.AddReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &emv1beta1.ExternalMetricValueList{
						Items: []emv1beta1.ExternalMetricValue{
							{
								MetricName: testMetricName,
								Value:      *testMetricValue,
								MetricLabels: map[string]string{
									testMetricLabels[0]: testMetricLabelsValues[0],
								},
								Timestamp: metav1.Now(),
							},
						},
					}, nil
				})
			},
			want: testMetricValue.AsApproximateFloat64(),
		},
		{
			name:  "Namespace and metric only",
			query: "namespace/" + testMetricName,
			setup: func(client *fakeemc.FakeExternalMetricsClient) {
				client.Fake.AddReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &emv1beta1.ExternalMetricValueList{
						Items: []emv1beta1.ExternalMetricValue{{
							MetricName: testMetricName,
							Value:      *testMetricValue,
							Timestamp:  metav1.Now(),
						}},
					}, nil
				})
			},
			want: testMetricValue.AsApproximateFloat64(),
		},
		{
			name:  "Metric only, default namespace",
			query: testMetricName,
			setup: func(client *fakeemc.FakeExternalMetricsClient) {
				client.Fake.AddReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &emv1beta1.ExternalMetricValueList{
						Items: []emv1beta1.ExternalMetricValue{{
							MetricName: testMetricName,
							Value:      *testMetricValue,
							Timestamp:  metav1.Now(),
						}},
					}, nil
				})
			},
			want: testMetricValue.AsApproximateFloat64(),
		},
		{
			name:    "Fails on invalid query",
			query:   "namespace/metric/extra",
			wantErr: true,
		},
		{
			name:  "Fails when external metrics API returns an error",
			query: testQuery,
			setup: func(client *fakeemc.FakeExternalMetricsClient) {
				client.Fake.AddReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("backend unavailable")
				})
			},
			wantErr: true,
		},
		{
			name:  "Fails when no external metrics are returned",
			query: testQuery,
			setup: func(client *fakeemc.FakeExternalMetricsClient) {
				client.Fake.AddReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &emv1beta1.ExternalMetricValueList{}, nil
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emclient := &fakeemc.FakeExternalMetricsClient{}
			if tt.setup != nil {
				tt.setup(emclient)
			}

			emp := &ExternalMetricsProvider{
				client: emclient,
			}

			got, err := emp.RunQuery(tt.query)
			if tt.wantErr {
				require.Error(t, err)
				assert.Zero(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExternalMetrics_IsOnline(t *testing.T) {
	emp := &ExternalMetricsProvider{
		client: &fakeemc.FakeExternalMetricsClient{},
	}

	online, err := emp.IsOnline()
	require.NoError(t, err)
	assert.True(t, online)
}
