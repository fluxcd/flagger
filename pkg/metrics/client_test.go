package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrometheusClient_RunQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	client, err := NewPrometheusClient(ts.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	query := `
        histogram_quantile(0.99,
          sum(
            rate(
              http_request_duration_seconds_bucket{
                kubernetes_namespace="test",
                kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
              }[1m]
            )
          ) by (le)
        )`

	val, err := client.RunQuery(query)
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 100 {
		t.Errorf("Got %v wanted %v", val, 100)
	}
}

func TestPrometheusClient_IsOnline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"config.file":"/etc/prometheus/prometheus.yml"}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	client, err := NewPrometheusClient(ts.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := client.IsOnline()
	if err != nil {
		t.Fatal(err.Error())
	}

	if !ok {
		t.Errorf("Got %v wanted %v", ok, true)
	}
}

func TestPrometheusClient_IsOffline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	client, err := NewPrometheusClient(ts.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := client.IsOnline()
	if err == nil {
		t.Errorf("Got no error wanted %v", http.StatusBadGateway)
	}

	if ok {
		t.Errorf("Got %v wanted %v", ok, false)
	}
}
