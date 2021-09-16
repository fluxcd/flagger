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

func TestNewDynatraceProvider(t *testing.T) {
	token := "token"
	cs := map[string][]byte{
		dynatraceAPITokenSecretKey: []byte(token),
	}

	mi := "100s"
	md, err := time.ParseDuration(mi)
	require.NoError(t, err)

	_, err = NewDynatraceProvider("100s", flaggerv1.MetricTemplateProvider{}, cs)
	require.Error(t, err)

	dp, err := NewDynatraceProvider("100s", flaggerv1.MetricTemplateProvider{Address: "https://mySampleEnv.live.dynatrace.com"}, cs)
	require.NoError(t, err)
	assert.Equal(t, "https://mySampleEnv.live.dynatrace.com/api/v2/metrics/query", dp.metricsQueryEndpoint)
	assert.Equal(t, "https://mySampleEnv.live.dynatrace.com/api/v2/metrics?pageSize=1", dp.apiValidationEndpoint)
	assert.Equal(t, int64(md.Milliseconds()*dynatraceDeltaMultiplierOnMetricInterval), dp.fromDelta)
	assert.Equal(t, token, dp.token)
}

func TestDynatraceProvider_RunQuery(t *testing.T) {
	token := "token"
	t.Run("ok", func(t *testing.T) {
		expected := 1.11111
		eq := `builtin:host.cpu.usage:filter(eq(Host,HOST-0990886B7D39FE29))`
		now := time.Now().Unix() * 1000
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			aq := r.URL.Query().Get("metricSelector")
			assert.Equal(t, eq, aq)
			assert.Equal(t, "Api-Token token", r.Header.Get(dynatraceAuthorizationHeaderKey))

			from, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
			if assert.NoError(t, err) {
				assert.Less(t, from, now)
			}

			to, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
			if assert.NoError(t, err) {
				assert.GreaterOrEqual(t, to, now)
			}

			resultTemplate := `
{
  "totalCount": 1,
  "nextPageKey": null,
  "result": [
    {
      "metricId": "builtin:host.cpu.usage",
      "data": [
	    {
		  "dimensions": [
            "HOST-0990886B7D39FE29"
          ],
          "timestamps": [
            1589455320000
          ],
          "values": [
            %f
          ]
        }
     ]
   }
  ]
}
`

			json := fmt.Sprintf(resultTemplate, expected)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		dp, err := NewDynatraceProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				dynatraceAPITokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)

		f, err := dp.RunQuery(eq)
		require.NoError(t, err)
		assert.Equal(t, expected, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := fmt.Sprintf(`{"series": [{"pointlist": []}]}`)
			w.Write([]byte(json))
		}))
		defer ts.Close()

		dp, err := NewDynatraceProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				dynatraceAPITokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}

func TestDynatraceProvider_IsOnline(t *testing.T) {
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
				assert.Equal(t, "Api-Token token", r.Header.Get(dynatraceAuthorizationHeaderKey))
				w.WriteHeader(c.code)
			}))
			defer ts.Close()

			dp, err := NewDynatraceProvider("1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					dynatraceAPITokenSecretKey: []byte(token),
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
