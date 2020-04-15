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

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
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

	kubeClient := fake.NewSimpleClientset(secret)

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

			if assert.Contains(t, r.Header, "Authorization") {

			}
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

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `{"status":"success","data":{"resultType":"vector","result":[]}}`
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

			if assert.Contains(t, r.Header, "Authorization") {

			}
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
