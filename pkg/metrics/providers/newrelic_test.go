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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewNewRelicProvider(t *testing.T) {
	queryKey := "query-key"
	accountId := "51312"
	cs := map[string][]byte{
		"newrelic_query_key":  []byte(queryKey),
		"newrelic_account_id": []byte(accountId),
	}

	duration := "100s"
	secondsDuration, err := time.ParseDuration(duration)
	require.NoError(t, err)

	nr, err := NewNewRelicProvider("100s", flaggerv1.MetricTemplateProvider{}, cs)
	require.NoError(t, err)
	assert.Equal(t, "https://insights-api.newrelic.com/v1/accounts/51312/query", nr.insightsQueryEndpoint)
	assert.Equal(t, int64(secondsDuration.Seconds()), nr.fromDelta)
	assert.Equal(t, queryKey, nr.queryKey)
}

func TestNewRelicProvider_RunQuery(t *testing.T) {
	queryKey := "query-key"
	accountId := "51312"
	t.Run("ok", func(t *testing.T) {
		q := `SELECT sum(nginx_ingress_controller_requests) / 1 FROM Metric WHERE status = '200'`
		eq := `SELECT sum(nginx_ingress_controller_requests) / 1 FROM Metric WHERE status = '200' SINCE 60 seconds ago`
		er := 1.11111
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			aq := r.URL.Query().Get("nrql")
			assert.Equal(t, eq, aq)
			assert.Equal(t, queryKey, r.Header.Get(newrelicQueryKeyHeaderKey))

			json := fmt.Sprintf(`{"results":[{"result": %f}]}`, er)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		nr, err := NewNewRelicProvider("1m",
			flaggerv1.MetricTemplateProvider{
				Address: ts.URL,
			},
			map[string][]byte{
				"newrelic_query_key":  []byte(queryKey),
				"newrelic_account_id": []byte(accountId),
			},
		)
		require.NoError(t, err)

		f, err := nr.RunQuery(q)
		assert.NoError(t, err)
		assert.Equal(t, er, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := fmt.Sprintf(`{"results": []}`)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		dp, err := NewNewRelicProvider(
			"1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				"newrelic_query_key":  []byte(queryKey),
				"newrelic_account_id": []byte(accountId)},
		)
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}

func TestNewReelicProvider_IsOnline(t *testing.T) {
	for _, c := range []struct {
		code        int
		errExpected bool
	}{
		{code: http.StatusOK, errExpected: false},
		{code: http.StatusUnauthorized, errExpected: true},
	} {
		t.Run(fmt.Sprintf("%d", c.code), func(t *testing.T) {
			queryKey := "query-key"
			accountId := "51312"
			query := `SELECT * FROM Metric SINCE 60 seconds ago`
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, queryKey, r.Header.Get(newrelicQueryKeyHeaderKey))
				assert.Equal(t, query, r.URL.Query().Get("nrql"))
				w.WriteHeader(c.code)
			}))
			defer ts.Close()

			dp, err := NewNewRelicProvider(
				"1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					"newrelic_query_key":  []byte(queryKey),
					"newrelic_account_id": []byte(accountId),
				},
			)
			require.NoError(t, err)

			_, err = dp.IsOnline()
			if c.errExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
