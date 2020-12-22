package observers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

func TestNginxObserver_GetRequestSuccessRate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expected := ` sum( rate( nginx_ingress_controller_requests{ namespace="nginx", ingress="podinfo", status!~"5.*" }[1m] ) ) / sum( rate( nginx_ingress_controller_requests{ namespace="nginx", ingress="podinfo" }[1m] ) ) * 100`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			promql := r.URL.Query()["query"][0]
			assert.Equal(t, expected, promql)

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
			Type:      "prometheus",
			Address:   ts.URL,
			SecretRef: nil,
		}, nil)
		require.NoError(t, err)

		observer := &NginxObserver{
			client: client,
		}

		val, err := observer.GetRequestSuccessRate(flaggerv1.MetricTemplateModel{
			Name:      "podinfo",
			Namespace: "nginx",
			Target:    "podinfo",
			Ingress:   "podinfo",
			Interval:  "1m",
		})
		require.NoError(t, err)

		assert.Equal(t, float64(100), val)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := `{"status":"success","data":{"resultType":"vector","result":[]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
			Type:      "prometheus",
			Address:   ts.URL,
			SecretRef: nil,
		}, nil)
		require.NoError(t, err)

		observer := &NginxObserver{
			client: client,
		}

		_, err = observer.GetRequestSuccessRate(flaggerv1.MetricTemplateModel{})
		require.True(t, errors.Is(err, providers.ErrNoValuesFound))
	})
}

func TestNginxObserver_GetRequestDuration(t *testing.T) {
	expected := ` sum( rate( nginx_ingress_controller_ingress_upstream_latency_seconds_sum{ namespace="nginx", ingress="podinfo" }[1m] ) ) / sum( rate( nginx_ingress_controller_ingress_upstream_latency_seconds_count{ namespace="nginx", ingress="podinfo" }[1m] ) ) * 1000`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promql := r.URL.Query()["query"][0]
		assert.Equal(t, expected, promql)

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	client, err := providers.NewPrometheusProvider(flaggerv1.MetricTemplateProvider{
		Type:      "prometheus",
		Address:   ts.URL,
		SecretRef: nil,
	}, nil)
	require.NoError(t, err)

	observer := &NginxObserver{
		client: client,
	}

	val, err := observer.GetRequestDuration(flaggerv1.MetricTemplateModel{
		Name:      "podinfo",
		Namespace: "nginx",
		Target:    "podinfo",
		Ingress:   "podinfo",
		Interval:  "1m",
	})
	require.NoError(t, err)

	assert.Equal(t, 100*time.Millisecond, val)
}
