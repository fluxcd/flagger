package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanaryObserver_GetDeploymentCounter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(json))
	}))
	defer ts.Close()

	observer := CanaryObserver{
		metricsServer: ts.URL,
	}

	val, err := observer.GetDeploymentCounter("podinfo", "default", "istio_requests_total", "1m")
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 100 {
		t.Errorf("Got %v wanted %v", val, 100)
	}

}

func TestCanaryObserver_GetDeploymentHistogram(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.596,"1"]}]}}`
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(json))
	}))
	defer ts.Close()

	observer := CanaryObserver{
		metricsServer: ts.URL,
	}

	val, err := observer.GetDeploymentHistogram("podinfo", "default", "istio_request_duration_seconds_bucket", "1m")
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 1 * time.Second {
		t.Errorf("Got %v wanted %v", val, 1 * time.Second)
	}
}