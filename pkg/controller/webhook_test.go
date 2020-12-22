package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
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

	err := CallWebhook("podinfo", v1.NamespaceDefault, flaggerv1.CanaryPhaseProgressing, hook)
	require.NoError(t, err)
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

	err := CallWebhook("podinfo", v1.NamespaceDefault, flaggerv1.CanaryPhaseProgressing, hook)
	assert.Error(t, err)
}

func TestCallEventWebhook(t *testing.T) {
	canaryName := "podinfo"
	canaryNamespace := v1.NamespaceDefault
	canaryMessage := fmt.Sprintf("Starting canary analysis for %s.%s", canaryName, canaryNamespace)
	canaryEventType := corev1.EventTypeNormal

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := json.NewDecoder(r.Body)

		var payload flaggerv1.CanaryWebhookPayload

		err := d.Decode(&payload)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if payload.Metadata["eventMessage"] != canaryMessage {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if payload.Metadata["eventType"] != canaryEventType {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if payload.Name != canaryName {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if payload.Namespace != canaryNamespace {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	canary := &flaggerv1.Canary{
		ObjectMeta: v1.ObjectMeta{
			Name:      canaryName,
			Namespace: canaryNamespace,
		},
		Status: flaggerv1.CanaryStatus{
			Phase: flaggerv1.CanaryPhaseProgressing,
		},
	}

	err := CallEventWebhook(canary, ts.URL, canaryMessage, canaryEventType)
	require.NoError(t, err)
}

func TestCallEventWebhookStatusCode(t *testing.T) {
	canaryName := "podinfo"
	canaryNamespace := v1.NamespaceDefault
	canaryMessage := fmt.Sprintf("Starting canary analysis for %s.%s", canaryName, canaryNamespace)
	canaryEventType := corev1.EventTypeNormal

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	canary := &flaggerv1.Canary{
		ObjectMeta: v1.ObjectMeta{
			Name:      canaryName,
			Namespace: canaryNamespace,
		},
		Status: flaggerv1.CanaryStatus{
			Phase: flaggerv1.CanaryPhaseProgressing,
		},
	}

	err := CallEventWebhook(canary, ts.URL, canaryMessage, canaryEventType)
	assert.Error(t, err)
}
