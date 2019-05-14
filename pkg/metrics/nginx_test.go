package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNginxObserver_GetRequestSuccessRate(t *testing.T) {
	expected := `sum(rate(nginx_ingress_controller_requests{namespace="nginx",ingress="podinfo",status!~"5.*"}[1m]))/sum(rate(nginx_ingress_controller_requests{namespace="nginx",ingress="podinfo"}[1m]))*100`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promql := r.URL.Query()["query"][0]
		if promql != expected {
			t.Errorf("\nGot %s \nWanted %s", promql, expected)
		}

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	client, err := NewPrometheusClient(ts.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	observer := &NginxObserver{
		client: client,
	}

	val, err := observer.GetRequestSuccessRate("podinfo", "nginx", "1m")
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 100 {
		t.Errorf("Got %v wanted %v", val, 100)
	}
}

func TestNginxObserver_GetRequestDuration(t *testing.T) {
	expected := `sum(rate(nginx_ingress_controller_ingress_upstream_latency_seconds_sum{namespace="nginx",ingress="podinfo"}[1m]))/sum(rate(nginx_ingress_controller_ingress_upstream_latency_seconds_count{namespace="nginx",ingress="podinfo"}[1m]))*1000`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promql := r.URL.Query()["query"][0]
		if promql != expected {
			t.Errorf("\nGot %s \nWanted %s", promql, expected)
		}

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	client, err := NewPrometheusClient(ts.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	observer := &NginxObserver{
		client: client,
	}

	val, err := observer.GetRequestDuration("podinfo", "nginx", "1m")
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 100*time.Millisecond {
		t.Errorf("Got %v wanted %v", val, 100*time.Millisecond)
	}
}
