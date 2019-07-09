package controller

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallWebhook(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()
	hook := flaggerv1.CanaryWebhook{
		Name:     "validation",
		URL:      ts.URL,
		Timeout:  "10s",
		Metadata: &map[string]string{"key1": "val1"},
	}

	err := CallWebhook("podinfo", "default", flaggerv1.CanaryPhaseProgressing, hook)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestCallWebhook_StatusCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	hook := flaggerv1.CanaryWebhook{
		Name: "validation",
		URL:  ts.URL,
	}

	err := CallWebhook("podinfo", "default", flaggerv1.CanaryPhaseProgressing, hook)
	if err == nil {
		t.Errorf("Got no error wanted %v", http.StatusInternalServerError)
	}
}
