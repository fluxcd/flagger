/*
Copyright 2025 The Flux authors

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
	"io"
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

type signozFakeClients struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
}

func signozFake() signozFakeClients {
	provider := flaggerv1.MetricTemplateProvider{
		Type:      "signoz",
		Address:   "http://signoz:3301",
		SecretRef: &corev1.LocalObjectReference{Name: "signoz"},
	}

	template := &flaggerv1.MetricTemplate{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "signoz",
		},
		Spec: flaggerv1.MetricTemplateSpec{
			Provider: provider,
			Query:    `{"requestType":"time_series"}`,
		},
	}

	flaggerClient := fakeFlagger.NewSimpleClientset(template)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "signoz",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test-signoz-token"),
		},
	}

	kubeClient := fake.NewSimpleClientset(secret)

	return signozFakeClients{
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
	}
}

func TestNewSignozProvider(t *testing.T) {
	clients := signozFake()

	template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
	require.NoError(t, err)

	secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
	require.NoError(t, err)

	sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
	require.NoError(t, err)

	assert.Equal(t, "http://signoz:3301", sp.url.String())
	assert.Equal(t, "test-signoz-token", sp.apiKey)
}

func TestSignozProvider_RunQuery(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expectedBody := `{"requestType":"time_series"}`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.True(t, strings.HasSuffix(r.URL.Path, "/api/v5/query_range"))

			// Verify SIGNOZ-API-KEY header
			apiKey := r.Header.Get("SIGNOZ-API-KEY")
			assert.Equal(t, "test-signoz-token", apiKey)

			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			assert.Equal(t, expectedBody, string(b))

			json := `{"data":{"result":[{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1742602572000,"value":"100"}]}],"queryName":"A"}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := signozFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)

		sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		val, err := sp.RunQuery(template.Spec.Query)
		require.NoError(t, err)
		assert.Equal(t, float64(100), val)
	})

	noResultTests := []struct {
		name        string
		queryResult string
	}{
		{name: "no values result", queryResult: `{"data":{"result":[]}}`},
		{name: "NaN result in series", queryResult: `{"data":{"result":[{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1714404069294,"value":"NaN"}]}],"queryName":"A"}]}}`},
		{name: "empty series", queryResult: `{"data":{"result":[{"series":[],"queryName":"A"}]}}`},
	}

	for _, tt := range noResultTests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.queryResult))
			}))
			defer ts.Close()

			clients := signozFake()

			template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
			require.NoError(t, err)
			template.Spec.Provider.Address = ts.URL

			secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
			require.NoError(t, err)

			sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
			require.NoError(t, err)

			_, err = sp.RunQuery(template.Spec.Query)
			require.True(t, errors.Is(err, ErrNoValuesFound))
		})
	}

	multipleResultTests := []struct {
		name        string
		queryResult string
	}{
		{name: "multiple series", queryResult: `{"data":{"result":[{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1714404069294,"value":"1"}]},{"labels":{},"labelString":"","values":[{"timestamp":1714404069294,"value":"2"}]}],"queryName":"A"}]}}`},
		{name: "multiple results", queryResult: `{"data":{"result":[{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1714404069294,"value":"1"}]}],"queryName":"A"},{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1714404069294,"value":"2"}]}],"queryName":"B"}]}}`},
	}

	for _, tt := range multipleResultTests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.queryResult))
			}))
			defer ts.Close()

			clients := signozFake()

			template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
			require.NoError(t, err)
			template.Spec.Provider.Address = ts.URL

			secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
			require.NoError(t, err)

			sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
			require.NoError(t, err)

			_, err = sp.RunQuery(template.Spec.Query)
			require.True(t, errors.Is(err, ErrMultipleValuesReturned))
		})
	}
}

func TestSignozProvider_IsOnline(t *testing.T) {
	t.Run("fail", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("bad gateway"))
		}))
		defer ts.Close()

		clients := signozFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)

		sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		ok, err := sp.IsOnline()
		assert.Error(t, err)
		assert.False(t, ok)
	})

	t.Run("ok", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `{"data":{"result":[{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1714404069294,"value":"1"}]}],"queryName":"F1"}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := signozFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)
		template.Spec.Provider.Address = ts.URL

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)

		sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		ok, err := sp.IsOnline()
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

func TestSignozProvider_RunQueryWithProviderHeaders(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expectedBody := `{"requestType":"time_series"}`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, []string{"tenant1"}, r.Header.Values("X-Scope-OrgID"))
			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			assert.Equal(t, expectedBody, string(b))
			json := `{"data":{"result":[{"series":[{"labels":{},"labelString":"","values":[{"timestamp":1742602572000,"value":"100"}]}],"queryName":"A"}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		clients := signozFake()

		template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)

		template.Spec.Provider.Address = ts.URL
		template.Spec.Provider.Headers = http.Header{
			"X-Scope-OrgID": []string{"tenant1"},
		}

		secret, err := clients.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "signoz", metav1.GetOptions{})
		require.NoError(t, err)

		sp, err := NewSignozProvider(template.Spec.Provider, secret.Data)
		require.NoError(t, err)

		val, err := sp.RunQuery(template.Spec.Query)
		require.NoError(t, err)
		assert.Equal(t, float64(100), val)
	})
}
