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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSkyWalkingProvider(t *testing.T) {
	sw, err := NewSkyWalkingProvider("1m", flaggerv1.MetricTemplateProvider{
		Address: "http://skywalking-oap.istio-system.svc.cluster.local:12800",
	}, nil)
	require.NoError(t, err)

	assert.Equal(t, 1*time.Minute, sw.interval)
}

func TestSkywalkingProvider_RunQuery(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `{"data":{"service_apdex0":{"values":{"values":[{"value":10000},{"value":10010}]}}}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		dp, err := NewSkyWalkingProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			nil,
		)
		require.NoError(t, err)

		f, err := dp.RunQuery(`{ "query": "query queryData($duration: Duration!) { service_apdex: readMetricsValues( condition: { name: \"service_apdex\", entity: { scope: Service, serviceName: \"agent::songs\", normal: true } }, duration: $duration) { label values { values { value } } } }" }`)
		require.NoError(t, err)

		assert.Equal(t, 10000.0, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `{"series": [{"pointlist": []}]}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		sw, err := NewSkyWalkingProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			nil,
		)
		require.NoError(t, err)
		_, err = sw.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}

func TestSkywalkingProvider_IsOnline(t *testing.T) {
	t.Run("ok if method not allowed", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}))
		defer ts.Close()

		sw, err := NewSkyWalkingProvider("1m", flaggerv1.MetricTemplateProvider{
			Address: ts.URL,
		}, nil)
		require.NoError(t, err)

		ok, err := sw.IsOnline()
		require.NoError(t, err)

		assert.True(t, ok)
	})

	t.Run("ok if 200", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := `healthy`
			w.Write([]byte(response))
		}))
		defer ts.Close()

		sw, err := NewSkyWalkingProvider("1m", flaggerv1.MetricTemplateProvider{
			Address: ts.URL,
		}, nil)
		require.NoError(t, err)

		ok, err := sw.IsOnline()
		require.NoError(t, err)

		assert.Equal(t, true, ok)
	})
}
