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
	"strings"
	"testing"
	"time"

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
		builderFunc        func() (*rest.Config, error)
		wantErr            bool
	}{
		{
			name:               "Custom provider address and token",
			Address:            testMetricServerAddress,
			InsecureSkipVerify: false,
			creds: map[string][]byte{
				"token": []byte("test-token"),
			},
			builderFunc: func() (*rest.Config, error) { return &rest.Config{}, nil },
			wantErr:     false,
		},
		{
			name:               "In cluster, automatic address and token",
			Address:            "",
			InsecureSkipVerify: true,
			creds:              map[string][]byte{},
			builderFunc: func() (*rest.Config, error) {
				return &rest.Config{
					Host:            "https://kubernetes.default.svc",
					BearerToken:     "fake-token",
					TLSClientConfig: rest.TLSClientConfig{Insecure: true},
				}, nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mtp := flaggerv1.MetricTemplateProvider{
				Address:            tt.Address,
				InsecureSkipVerify: tt.InsecureSkipVerify,
			}
			emp, err := newExternalMetricsProviderWithBuilder(mtp, tt.creds, tt.builderFunc)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, emp)
			assert.Equal(t, 5*time.Second, emp.timeout)
		})
	}
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
	fakeExternalMetricsClient := fakeemc.FakeExternalMetricsClient{}
	fakeExternalMetricsClient.Fake.AddReactor("list", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
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

	emp := &ExternalMetricsProvider{
		timeout: 5 * time.Second,
		client:  &fakeExternalMetricsClient,
	}

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "Full query with label selector",
			query: testQuery,
		},
		{
			name:  "Namespace and metric only",
			query: "namespace/" + testMetricName,
		},
		{
			name:  "Metric only, default namespace",
			query: testMetricName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := emp.RunQuery(tt.query)
			require.NoError(t, err)
			assert.Equal(t, testMetricValue.AsApproximateFloat64(), f)
		})
	}
}

func TestExternalMetrics_IsOnline(t *testing.T) {
	emp := &ExternalMetricsProvider{
		timeout: 5 * time.Second,
		client:  &fakeemc.FakeExternalMetricsClient{},
	}

	online, err := emp.IsOnline()
	require.NoError(t, err)
	assert.True(t, online)
}
