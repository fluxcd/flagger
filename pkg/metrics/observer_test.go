package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanaryObserver_GetDeploymentCounter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	observer := Observer{
		metricsServer: ts.URL,
	}

	val, err := observer.GetIstioSuccessRate("podinfo", "default", "istio_requests_total", "1m")
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 100 {
		t.Errorf("Got %v wanted %v", val, 100)
	}

}

func TestCanaryObserver_GetDeploymentHistogram(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.596,"0.2"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	observer := Observer{
		metricsServer: ts.URL,
	}

	val, err := observer.GetIstioRequestDuration("podinfo", "default", "istio_request_duration_seconds_bucket", "1m")
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 200*time.Millisecond {
		t.Errorf("Got %v wanted %v", val, 200*time.Millisecond)
	}
}

func TestCheckMetricsServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"config.file":"/etc/prometheus/prometheus.yml"}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	ok, err := CheckMetricsServer(ts.URL)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !ok {
		t.Errorf("Got %v wanted %v", ok, true)
	}
}

func TestCheckMetricsServer_Offline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	ok, err := CheckMetricsServer(ts.URL)
	if err == nil {
		t.Errorf("Got no error wanted %v", http.StatusBadGateway)
	}

	if ok {
		t.Errorf("Got %v wanted %v", ok, false)
	}
}
