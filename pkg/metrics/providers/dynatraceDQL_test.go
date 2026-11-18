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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewDynatraceDQLProvider(t *testing.T) {
	token := "token"
	cs := map[string][]byte{
		dynatraceDQLAPITokenSecretKey: []byte(token),
	}

	mi := "-100s"
	md, err := time.ParseDuration(mi)
	require.NoError(t, err)

	_, err = NewDynatraceDQLProvider("100s", flaggerv1.MetricTemplateProvider{}, cs)
	require.Error(t, err)

	dp, err := NewDynatraceDQLProvider("100s", flaggerv1.MetricTemplateProvider{Address: "https://mySampleEnv.apps.dynatrace.com"}, cs)
	require.NoError(t, err)
	assert.Equal(t, "https://mySampleEnv.apps.dynatrace.com/platform/storage/query/v1", dp.apiRoot)
	assert.Equal(t, md, dp.fromDelta)
	assert.Equal(t, token, dp.token)
}

func TestDynatraceDQLProvider_RunQuery(t *testing.T) {
	token := "token"
	t.Run("ok", func(t *testing.T) {
		expected := 1.11111
		query := `timeseries{cpu=avg(dt.host.cpu.usage),filter:matchesValue(dt.smartscape.host,"HOST-001109335619D5DD")},from:now()-5m|fields r=arraySum(cpu)`
		queryToken := "query token"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				assert.Equal(t, r.URL.Path, "/platform/storage/query/v1/query:execute")
				b, err := io.ReadAll(r.Body)
				assert.Nil(t, err, "read body error not nil, %w", err)
				assert.Contains(t, string(b), strings.Replace(query, `"`, `\"`, -1))
				assert.Equal(t, "Bearer token", r.Header.Get(dynatraceDQLAuthorizationHeaderKey))

				resultTemplate := `{"state":"RUNNING","requestToken":"%s","ttlSeconds":399}`

				json := fmt.Sprintf(resultTemplate, queryToken)
				w.Write([]byte(json))
			case http.MethodGet:
				assert.Equal(t, r.URL.Path, "/platform/storage/query/v1/query:poll")
				assert.Equal(t, "Bearer token", r.Header.Get(dynatraceDQLAuthorizationHeaderKey))
				reqToken := r.URL.Query().Get("request-token")
				assert.Equal(t, queryToken, reqToken)

				// metadata has stuff, but we don't care for this testing
				resultTemplate := `
{
    "state": "SUCCEEDED",
    "progress": 100,
    "result": {
        "records": [
            {
                "r": %f
            }
        ],
        "types": [
            {
                "indexRange": [
                    0,
                    0
                ],
                "mappings": {
                    "r": {
                        "type": "double"
                    }
                }
            }
        ],
        "metadata": { }
    }
}
`

				json := fmt.Sprintf(resultTemplate, expected)
				w.Write([]byte(json))
			default:
				assert.Fail(t, "dynatrace DQL should not be calling other methods")
			}
		}))
		defer ts.Close()

		dp, err := NewDynatraceDQLProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				dynatraceDQLAPITokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)

		f, err := dp.RunQuery(query)
		require.NoError(t, err)
		assert.Equal(t, expected, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `
{
    "state": "SUCCEEDED",
    "progress": 100,
    "result": {
        "records": [ ],
        "types": [ ],
        "metadata": { }
    }
}
`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		dp, err := NewDynatraceDQLProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			map[string][]byte{
				dynatraceDQLAPITokenSecretKey: []byte(token),
			},
		)
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}

func TestDynatraceDQLProvider_IsOnline(t *testing.T) {
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
				assert.Equal(t, "Bearer token", r.Header.Get(dynatraceDQLAuthorizationHeaderKey))
				w.WriteHeader(c.code)
				json := fmt.Sprintf(`{"valid": %t}`, !c.errExpected)
				w.Write([]byte(json))
			}))
			defer ts.Close()

			dp, err := NewDynatraceDQLProvider("1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					dynatraceDQLAPITokenSecretKey: []byte(token),
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
