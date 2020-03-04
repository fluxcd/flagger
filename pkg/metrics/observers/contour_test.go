package observers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

func TestContourObserver_GetRequestSuccessRate(t *testing.T) {
	expected := ` sum( rate( envoy_cluster_upstream_rq{ envoy_cluster_name=~"default_podinfo-canary_[0-9a-zA-Z-]+", envoy_response_code!~"5.*" }[1m] ) ) / sum( rate( envoy_cluster_upstream_rq{ envoy_cluster_name=~"default_podinfo-canary_[0-9a-zA-Z-]+", }[1m] ) ) * 100`

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

	observer := &ContourObserver{
		client: client,
	}

	val, err := observer.GetRequestSuccessRate(flaggerv1.MetricTemplateModel{
		Name:      "podinfo",
		Namespace: "default",
		Target:    "podinfo",
		Service:   "podinfo",
		Interval:  "1m",
	})
	require.NoError(t, err)

	assert.Equal(t, float64(100), val)
}

func TestContourObserver_GetRequestDuration(t *testing.T) {
	expected := ` histogram_quantile( 0.99, sum( rate( envoy_cluster_upstream_rq_time_bucket{ envoy_cluster_name=~"default_podinfo-canary_[0-9a-zA-Z-]+", }[1m] ) ) by (le) )`

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

	observer := &ContourObserver{
		client: client,
	}

	val, err := observer.GetRequestDuration(flaggerv1.MetricTemplateModel{
		Name:      "podinfo",
		Namespace: "default",
		Target:    "podinfo",
		Service:   "podinfo",
		Interval:  "1m",
	})
	require.NoError(t, err)

	assert.Equal(t, 100*time.Millisecond, val)
}
