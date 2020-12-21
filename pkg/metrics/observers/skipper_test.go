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

package observers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/metrics/providers"
)

func TestSkipperObserver_GetRequestSuccessRate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expected := ` sum(rate(skipper_response_duration_seconds_bucket{route=~"kube(ew)?_skipper__skipper_ingress_canary__.*__backend_canary(_[0-9]+)?",code!~"5..",le="+Inf"}[1m])) / sum(rate(skipper_response_duration_seconds_bucket{route=~"kube(ew)?_skipper__skipper_ingress_canary__.*__backend_canary(_[0-9]+)?",le="+Inf"}[1m])) * 100`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, expected, promql)

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
			Type:      "prometheus",
			Address:   ts.URL,
			SecretRef: nil,
		}, nil)
		require.NoError(t, err)

		observer := &SkipperObserver{client: client}
		val, err := observer.GetRequestSuccessRate(flaggerv1.MetricTemplateModel{
			Namespace: "skipper",
			Interval:  "1m",
			Service:   "backend",
			Ingress:   "skipper-ingress",
		})
		require.NoError(t, err)

		assert.Equal(t, float64(100), val)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `{"status":"success","data":{"resultType":"vector","result":[]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
			Type:      "prometheus",
			Address:   ts.URL,
			SecretRef: nil,
		}, nil)
		require.NoError(t, err)

		observer := &SkipperObserver{client: client}
		_, err = observer.GetRequestSuccessRate(flaggerv1.MetricTemplateModel{})
		require.True(t, errors.Is(err, providers.ErrNoValuesFound))
	})
}

func TestSkipperObserver_GetRequestDuration(t *testing.T) {
	expected := ` sum(rate(skipper_serve_route_duration_seconds_sum{route=~"kube(ew)?_skipper__skipper_ingress_canary__.*__backend_canary(_[0-9]+)?"}[1m])) / sum(rate(skipper_serve_route_duration_seconds_count{route=~"kube(ew)?_skipper__skipper_ingress_canary__.*__backend_canary(_[0-9]+)?"}[1m])) * 1000`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promql := r.URL.Query()["query"][0]
		assert.Equal(t, expected, promql)

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
		Type:      "prometheus",
		Address:   ts.URL,
		SecretRef: nil,
	}, nil)
	require.NoError(t, err)

	observer := &SkipperObserver{client: client}
	val, err := observer.GetRequestDuration(flaggerv1.MetricTemplateModel{
		Namespace: "skipper",
		Interval:  "1m",
		Service:   "backend",
		Ingress:   "skipper-ingress",
	})
	require.NoError(t, err)

	assert.Equal(t, 100*time.Millisecond, val)
}
