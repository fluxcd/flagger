/*
Copyright 2026 The Flux authors

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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func ampAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
}

func TestNewAmpProvider(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ampAWSEnv(t)
		p, err := NewAmpProvider(flaggerv1.MetricTemplateProvider{
			Type:    "amp",
			Address: "https://aps-workspaces.us-west-2.amazonaws.com/workspaces/ws-example",
			Region:  "us-west-2",
		})
		require.NoError(t, err)
		assert.Equal(t, "https://aps-workspaces.us-west-2.amazonaws.com/workspaces/ws-example", p.url.String())
	})

	t.Run("missing region", func(t *testing.T) {
		_, err := NewAmpProvider(flaggerv1.MetricTemplateProvider{
			Type:    "amp",
			Address: "https://aps-workspaces.us-west-2.amazonaws.com/workspaces/ws-example",
		})
		require.Error(t, err)
		assert.EqualError(t, err, "region not specified")
	})

	t.Run("invalid address", func(t *testing.T) {
		_, err := NewAmpProvider(flaggerv1.MetricTemplateProvider{
			Type:   "amp",
			Region: "us-west-2",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "address")
	})
}

func TestAmpProvider_RunQuery(t *testing.T) {
	ampAWSEnv(t)
	query := `sum(envoy_cluster_upstream_rq)`

	t.Run("ok", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, query, promql)

			auth := r.Header.Get("Authorization")
			assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256"), "Authorization header %q", auth)

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		p, err := NewAmpProvider(flaggerv1.MetricTemplateProvider{
			Type:    "amp",
			Address: ts.URL,
			Region:  "us-west-2",
		})
		require.NoError(t, err)

		val, err := p.RunQuery(query)
		require.NoError(t, err)
		assert.Equal(t, float64(100), val)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256"), "Authorization header %q", auth)

			json := `{"status":"success","data":{"resultType":"vector","result":[]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		p, err := NewAmpProvider(flaggerv1.MetricTemplateProvider{
			Type:    "amp",
			Address: ts.URL,
			Region:  "us-west-2",
		})
		require.NoError(t, err)

		_, err = p.RunQuery(query)
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}
