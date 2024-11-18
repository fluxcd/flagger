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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewSplunkProvider(t *testing.T) {
	token := "token"
	cs := map[string][]byte{
		signalFxTokenSecretKey: []byte(token),
	}

	mi := "100s"
	md, err := time.ParseDuration(mi)
	require.NoError(t, err)

	sp, err := NewSplunkProvider("100s", flaggerv1.MetricTemplateProvider{Address: "https://api.us1.signalfx.com"}, cs)
	require.NoError(t, err)
	assert.Equal(t, "https://api.us1.signalfx.com/v1/timeserieswindow", sp.metricsQueryEndpoint)
	assert.Equal(t, "https://api.us1.signalfx.com/v2/metric?limit=1", sp.apiValidationEndpoint)
	assert.Equal(t, int64(md.Seconds()*signalFxFromDeltaMultiplierOnMetricInterval), sp.fromDelta)
	assert.Equal(t, token, sp.token)
}

func TestSplunkProvider_RunQuery(t *testing.T) {
	token := "token"
	t.Run("ok", func(t *testing.T) {
		expected := 1.11111
		eq := `sf_metric:service.request.count AND http_status_code:*`
		now := time.Now().Unix()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			aq := r.URL.Query().Get("query")
			assert.Equal(t, eq, aq)
			assert.Equal(t, token, r.Header.Get(signalFxTokenHeaderKey))

			from, err := strconv.ParseInt(r.URL.Query().Get("startMS"), 10, 64)
			if assert.NoError(t, err) {
				assert.Less(t, from, now)
			}

			to, err := strconv.ParseInt(r.URL.Query().Get("endMS"), 10, 64)
			if assert.NoError(t, err) {
				assert.GreaterOrEqual(t, to, now)
			}

			json := fmt.Sprintf(`{"data":{"AAAAAAAAAAA":[[1731643210000,%f]]},"errors":[]}`, expected)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		sp, err := NewSplunkProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				signalFxTokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)

		f, err := sp.RunQuery(eq)
		require.NoError(t, err)
		assert.Equal(t, expected, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := fmt.Sprintf(`{"data": {}, "errors": []}`)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		sp, err := NewSplunkProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				signalFxTokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)
		_, err = sp.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})

	t.Run("multiple values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := fmt.Sprintf(`{"data":{"AAAAAAAAAAA":[[1731643210000,6]],"AAAAAAAAAAE":[[1731643210000,6]]},"errors":[]}`)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		sp, err := NewSplunkProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				signalFxTokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)
		_, err = sp.RunQuery("")
		require.True(t, errors.Is(err, ErrMultipleValuesReturned))
	})
}

func TestSplunkProvider_IsOnline(t *testing.T) {
	for _, c := range []struct {
		code        int
		errExpected bool
	}{
		{code: http.StatusOK, errExpected: false},
		{code: http.StatusUnauthorized, errExpected: true},
	} {
		t.Run(fmt.Sprintf("%d", c.code), func(t *testing.T) {
			token := "token"
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, token, r.Header.Get(signalFxTokenHeaderKey))
				w.WriteHeader(c.code)
			}))
			defer ts.Close()

			sp, err := NewSplunkProvider("1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					signalFxTokenSecretKey: []byte(token),
				},
			)
			require.NoError(t, err)

			_, err = sp.IsOnline()
			if c.errExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
