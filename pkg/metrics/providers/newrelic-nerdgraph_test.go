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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewNerdGraphProvider(t *testing.T) {
	apiKey := "api-key"
	accountID := "12345"

	t.Run("default host", func(t *testing.T) {
		cs := map[string][]byte{
			"newrelic_api_key":    []byte(apiKey),
			"newrelic_account_id": []byte(accountID),
		}

		provider, err := NewNerdGraphProvider("1m", flaggerv1.MetricTemplateProvider{}, cs)
		require.NoError(t, err)
		assert.Equal(t, nerdGraphDefaultHost, provider.endpoint)
		assert.Equal(t, apiKey, provider.apiKey)
	})

	t.Run("custom host", func(t *testing.T) {
		cs := map[string][]byte{
			"newrelic_api_key":    []byte(apiKey),
			"newrelic_account_id": []byte(accountID),
		}
		customHost := "https://my-custom.newrelic.com/graphql"

		provider, err := NewNerdGraphProvider("1m", flaggerv1.MetricTemplateProvider{Address: customHost}, cs)
		require.NoError(t, err)
		assert.Equal(t, customHost, provider.endpoint)
	})

	t.Run("missing credentials", func(t *testing.T) {
		cs := map[string][]byte{}
		_, err := NewNerdGraphProvider("1m", flaggerv1.MetricTemplateProvider{}, cs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), nerdGraphAPIKeySecretKey)
	})
}

func TestNerdGraphProvider_RunQuery(t *testing.T) {
	apiKey := "api-key"
	accountID := "12345"

	standardCredentials := map[string][]byte{
		"newrelic_api_key":    []byte(apiKey),
		"newrelic_account_id": []byte(accountID),
	}

	t.Run("ok", func(t *testing.T) {
		nrqlQuery := "SELECT average(duration) FROM Transaction"
		expectedResult := 1.11111
		expectedGQLSubstring := fmt.Sprintf("account(id: %s)", accountID)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check headers
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, apiKey, r.Header.Get(nerdGraphAPIKeyHeaderKey))
			assert.Equal(t, nerdGraphContentTypeHeader, r.Header.Get("Content-Type"))

			var reqBody nerdGraphPayload
			b, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			err = json.Unmarshal(b, &reqBody)
			require.NoError(t, err)

			// Assert that the sent query contains the necessary parts
			assert.Contains(t, reqBody.Query, expectedGQLSubstring, "Payload should contain account ID in template")
			assert.Contains(t, reqBody.Query, "query ($query: Nrql!)", "Payload should define the NRQL variable")

			// Check that the final NRQL is fully formed and placed in the variables section
			finalNRQL, ok := reqBody.Variables["query"].(string)
			require.True(t, ok, "Variables should contain 'query' as a string")
			assert.Contains(t, finalNRQL, nrqlQuery, "Final NRQL in variables should contain the raw query")
			assert.Contains(t, finalNRQL, "SINCE 60 SECONDS ago", "Final NRQL in variables should contain time delta")

			jsonResp := fmt.Sprintf(`{"data": {"actor": {"account": {"nrql": {"results": [{"average": %f}]}}}}}`, expectedResult)
			w.Write([]byte(jsonResp))
		}))
		defer ts.Close()

		provider, err := NewNerdGraphProvider("1m",
			flaggerv1.MetricTemplateProvider{
				Address: ts.URL,
			},
			standardCredentials,
		)
		require.NoError(t, err)

		val, err := provider.RunQuery(nrqlQuery)
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, val)
	})

	t.Run("no values found", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data": {"actor": {"account": {"nrql": {"results": []}}}}}`))
		}))
		defer ts.Close()

		provider, err := NewNerdGraphProvider(
			"1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			standardCredentials,
		)
		require.NoError(t, err)
		_, err = provider.RunQuery("SELECT * FROM X")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})

	t.Run("no numeric value in result", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data": {"actor": {"account": {"nrql": {"results": [{"string": "not-a-float"}]}}}}}`))
		}))
		defer ts.Close()

		provider, err := NewNerdGraphProvider(
			"1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			standardCredentials,
		)
		require.NoError(t, err)
		_, err = provider.RunQuery("SELECT * FROM X")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})

	t.Run("graphql error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"errors": [{"message": "Invalid query"}]}`))
		}))
		defer ts.Close()

		provider, err := NewNerdGraphProvider(
			"1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			standardCredentials,
		)
		require.NoError(t, err)
		_, err = provider.RunQuery("SELECT * FROM X")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid query")
	})

	t.Run("http error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer ts.Close()

		provider, err := NewNerdGraphProvider(
			"1m",
			flaggerv1.MetricTemplateProvider{Address: ts.URL},
			standardCredentials,
		)
		require.NoError(t, err)
		_, err = provider.RunQuery("SELECT * FROM X")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error response")
	})
}

func TestNerdGraphProvider_IsOnline(t *testing.T) {
	apiKey := "api-key"
	accountID := "12345"
	pingQuery := "{ actor { user { name } } }"

	type testCase struct {
		name        string
		code        int
		body        string
		errExpected bool
	}

	for _, c := range []testCase{
		{name: "ok", code: http.StatusOK, body: `{"data": {"actor": {"user": {"name": "test"}}}}`, errExpected: false},
		{name: "http error", code: http.StatusUnauthorized, body: ``, errExpected: true},
		{name: "graphql error", code: http.StatusOK, body: `{"errors": [{"message": "Invalid API Key"}]}`, errExpected: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, apiKey, r.Header.Get(nerdGraphAPIKeyHeaderKey))

				var reqBody nerdGraphQuery
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				// Only unmarshal if body is not empty (which it might be for a simple ping)
				if len(b) > 0 {
					err = json.Unmarshal(b, &reqBody)
					require.NoError(t, err)
					assert.Equal(t, pingQuery, reqBody.Query)
				}

				w.WriteHeader(c.code)
				if c.body != "" {
					w.Write([]byte(c.body))
				}
			}))
			defer ts.Close()

			provider, err := NewNerdGraphProvider(
				"1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					"newrelic_api_key":    []byte(apiKey),
					"newrelic_account_id": []byte(accountID),
				},
			)
			require.NoError(t, err)

			_, err = provider.IsOnline()
			if c.errExpected {
				require.Error(t, err)
				if c.name == "graphql error" {
					assert.Contains(t, err.Error(), "Invalid API Key")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Helper to test the recursive findResultValue function directly
func Test_findResultValue(t *testing.T) {
	t.Run("finds nested result", func(t *testing.T) {
		data := map[string]any{
			"actor": map[string]any{
				"account": map[string]any{
					"nrql": map[string]any{
						"results": []any{
							map[string]any{
								"average": 123.45,
								"other":   "foo",
							},
						},
					},
				},
			},
		}
		val, err := findResultValue(data)
		require.NoError(t, err)
		assert.Equal(t, 123.45, val)
	})

	t.Run("returns error for empty results", func(t *testing.T) {
		data := map[string]any{
			"actor": map[string]any{
				"nrql": map[string]any{
					"results": []any{},
				},
			},
		}
		_, err := findResultValue(data)
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})

	t.Run("returns error for no numeric value", func(t *testing.T) {
		data := map[string]any{
			"results": []any{
				map[string]any{
					"string": "foo",
					"bool":   true,
				},
			},
		}
		_, err := findResultValue(data)
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}
