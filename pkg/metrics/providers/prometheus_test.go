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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
)

type fakeClients struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
}

func prometheusFake() fakeClients {
	provider := flaggerv1.MetricTemplateProvider{
		Type:      "prometheus",
		Address:   "http://prometheus:9090",
		SecretRef: &corev1.LocalObjectReference{Name: "prometheus"},
	}

	template := &flaggerv1.MetricTemplate{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "prometheus",
		},
		Spec: flaggerv1.MetricTemplateSpec{
			Provider: provider,
			Query:    "sum(envoy_cluster_upstream_rq)",
		},
	}

	flaggerClient := fakeFlagger.NewSimpleClientset(template)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "prometheus",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("username"),
			"password": []byte("password"),
		},
	}

	bearerTokenSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "prometheus-bearer",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("bearer_token"),
		},
	}

	kubeClient := fake.NewSimpleClientset(secret, bearerTokenSecret)

	return fakeClients{
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
	}
}

func TestNewPrometheusProvider(t *testing.T) {
	clients := prometheusFake()

	template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
	require.NoError(t, err)

	secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
	require.NoError(t, err)

	prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
	require.NoError(t, err)

	assert.Equal(t, "http://prometheus:9090", prom.url.String())
	assert.Equal(t, "password", prom.password)
}

func TestPrometheusProvider_RunQueryWithBasicAuth(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expected := `sum(envoy_cluster_upstream_rq)`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, expected, promql)

			header, ok := r.Header["Authorization"]
			if assert.True(t, ok, "Authorization header not found") {
				assert.True(t, strings.Contains(header[0], "Basic"), "Basic authorization header not found")
			}

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := prometheusFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)

		prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		val, err := prom.RunQuery(template.Spec.Query)
		require.NoError(t, err)

		assert.Equal(t, float64(100), val)
	})

	noResultTests := []struct {
		name        string
		queryResult string
	}{
		{name: "no values result", queryResult: `{"status":"success","data":{"resultType":"vector","result":[]}}`},
		{name: "NaN result", queryResult: `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1643023250.379,"NaN"]}]}}`},
	}

	for _, tt := range noResultTests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json := tt.queryResult
				w.Write([]byte(json))
			}))
			defer ts.Close()

			clients := prometheusFake()

			template, err := clients.flaggerClient.FlaggerV1beta1().
				MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
			require.NoError(t, err)
			template.Spec.Provider.Address = ts.URL

			secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
			require.NoError(t, err)

			prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
			require.NoError(t, err)

			_, err = prom.RunQuery(template.Spec.Query)
			require.True(t, errors.Is(err, ErrNoValuesFound))
		})
	}

	multipleResultTests := []struct {
		name        string
		queryResult string
	}{
		{name: "values instead of value", queryResult: `{"status": "success","data": {"resultType": "matrix","result": [{"metric": {"__name__": "processTime_seconds:avg"},"values": [[1714404069.294,"NaN"],[1714404071.3,"NaN"],[1714404099.294,"NaN"],[1714404101.3,"NaN"]]},{"metric": {"__name__": "processTime_seconds:avg"},"values": [[1714404069.294,"NaN"],[1714404071.3,"NaN"],[1714404099.294,"NaN"],[1714404101.3,"NaN"]]}]}}`},
	}

	for _, tt := range multipleResultTests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json := tt.queryResult
				w.Write([]byte(json))
			}))
			defer ts.Close()

			clients := prometheusFake()

			template, err := clients.flaggerClient.FlaggerV1beta1().
				MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
			require.NoError(t, err)
			template.Spec.Provider.Address = ts.URL

			secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
			require.NoError(t, err)

			prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
			require.NoError(t, err)

			_, err = prom.RunQuery(template.Spec.Query)
			require.True(t, errors.Is(err, ErrMultipleValuesReturned))
		})
	}

}

func TestPrometheusProvider_RunQueryWithBearerAuth(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expected := `sum(envoy_cluster_upstream_rq)`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, expected, promql)

			header, ok := r.Header["Authorization"]
			if assert.True(t, ok, "Authorization header not found") {
				assert.True(t, strings.Contains(header[0], "Bearer"), "Bearer authorization header not found")
			}

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := prometheusFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus-bearer", metav1.GetOptions{})
		require.NoError(t, err)

		prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		val, err := prom.RunQuery(template.Spec.Query)
		require.NoError(t, err)

		assert.Equal(t, float64(100), val)
	})
}

func TestPrometheusProvider_IsOnline(t *testing.T) {
	t.Run("fail", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer ts.Close()

		clients := prometheusFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL
		template.Spec.Provider.SecretRef = nil

		prom, err := NewPrometheusProvider(template.Spec.Provider, nil)
		require.NoError(t, err)

		ok, err := prom.IsOnline()
		assert.Error(t, err, "Got no error wanted %v", http.StatusBadGateway)
		assert.False(t, ok)
	})

	t.Run("ok", func(t *testing.T) {
		expected := `vector(1)`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, expected, promql)

			header, ok := r.Header["Authorization"]
			if assert.True(t, ok, "Authorization header not found") {
				assert.True(t, strings.Contains(header[0], "Basic"), "Basic authorization header not found")
			}

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"1"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := prometheusFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)

		prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		ok, err := prom.IsOnline()
		require.NoError(t, err)

		assert.Equal(t, true, ok)
	})
}

func TestPrometheusProvider_RunQueryWithProviderHeaders(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expected := `sum(envoy_cluster_upstream_rq)`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, expected, promql)

			assert.Equal(t, []string{"tenant1"}, r.Header.Values("X-Scope-Orgid"))

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := prometheusFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)

		template.Spec.Provider.Address = ts.URL
		template.Spec.Provider.Headers = http.Header{
			"X-Scope-OrgID": []string{"tenant1"},
		}

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "prometheus", metav1.GetOptions{})
		require.NoError(t, err)

		prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		val, err := prom.RunQuery(template.Spec.Query)
		require.NoError(t, err)

		assert.Equal(t, float64(100), val)
	})
}
