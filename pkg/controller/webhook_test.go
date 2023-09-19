/*
Copyright 2020 The Flux authors

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

type testRequest struct {
	path   string
	body   map[string]any
	header http.Header
}

func TestCallWebhook(t *testing.T) {
	requests := []testRequest{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		requests = append(requests, testRequest{
			path: r.URL.Path,
			body: body,
		})

	}))
	defer ts.Close()

	hook := flaggerv1.CanaryWebhook{
		Name:     "validation",
		URL:      ts.URL + "/testing",
		Timeout:  "10s",
		Metadata: &map[string]string{"key1": "val1"},
	}

	canary := flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name: "podinfo", Namespace: corev1.NamespaceDefault,
		},
		Status: flaggerv1.CanaryStatus{
			TrackedConfigs: &map[string]string{
				"test-config-map": "484637c76acaa7c6",
			},
			LastAppliedSpec: "4cb74184589",
		},
	}
	err := CallWebhook(canary,
		flaggerv1.CanaryPhaseProgressing, hook)
	require.NoError(t, err)

	want := []testRequest{
		{
			path: "/testing",
			body: map[string]any{
				"name":      "podinfo",
				"namespace": "default",
				"phase":     "Progressing",
				"checksum":  canaryChecksum(canary),
				"metadata": map[string]any{
					"key1": "val1",
				},
			},
		},
	}
	require.EqualValues(t, want, requests)
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

	err := CallWebhook(
		flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{
				Name: "podinfo", Namespace: corev1.NamespaceDefault}},
		flaggerv1.CanaryPhaseProgressing, hook)
	assert.Error(t, err)
}

func TestCallEventWebhook(t *testing.T) {
	canaryName := "podinfo"
	canaryNamespace := corev1.NamespaceDefault
	canaryMessage := fmt.Sprintf("Starting canary analysis for %s.%s", canaryName, canaryNamespace)
	canaryEventType := corev1.EventTypeNormal

	canary := &flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canaryName,
			Namespace: canaryNamespace,
		},
		Status: flaggerv1.CanaryStatus{
			Phase: flaggerv1.CanaryPhaseProgressing,
		},
	}

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

		if payload.Checksum != canaryChecksum(*canary) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	hook := flaggerv1.CanaryWebhook{
		Name: "event",
		URL:  ts.URL,
	}

	err := CallEventWebhook(canary, hook, canaryMessage, canaryEventType)
	require.NoError(t, err)
}

func TestCallEventWebhookStatusCode(t *testing.T) {
	canaryName := "podinfo"
	canaryNamespace := corev1.NamespaceDefault
	canaryMessage := fmt.Sprintf("Starting canary analysis for %s.%s", canaryName, canaryNamespace)
	canaryEventType := corev1.EventTypeNormal

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	hook := flaggerv1.CanaryWebhook{
		Name: "event",
		URL:  ts.URL,
	}
	canary := &flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      canaryName,
			Namespace: canaryNamespace,
		},
		Status: flaggerv1.CanaryStatus{
			Phase: flaggerv1.CanaryPhaseProgressing,
		},
	}

	err := CallEventWebhook(canary, hook, canaryMessage, canaryEventType)
	assert.Error(t, err)
}

func TestCanaryChecksum(t *testing.T) {
	canary1 := flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name: "podinfo", Namespace: corev1.NamespaceDefault},

		Status: flaggerv1.CanaryStatus{
			TrackedConfigs: &map[string]string{
				"test-config-map": "484637c76acaa7c6",
			},
			LastAppliedSpec: "5f56684589",
		},
	}
	canary1sum := canaryChecksum(canary1)

	canary2 := flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name: "podinfo", Namespace: corev1.NamespaceDefault},

		Status: flaggerv1.CanaryStatus{
			TrackedConfigs: &map[string]string{
				"test-config-map": "9fc3a7c76acaa7c6",
			},
			LastAppliedSpec: "5f56684589",
		},
	}
	canary2sum := canaryChecksum(canary2)

	canary3 := flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podinfo",
			Namespace: corev1.NamespaceDefault,
		},
		Status: flaggerv1.CanaryStatus{
			TrackedConfigs: &map[string]string{
				"test-config-map": "484637c76acaa7c6",
			},
			LastAppliedSpec: "4cb74184589",
		},
	}
	canary3sum := canaryChecksum(canary3)

	canary4 := flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podinfo",
			Namespace: corev1.NamespaceDefault,
		},
		Status: flaggerv1.CanaryStatus{
			TrackedConfigs:  nil,
			LastAppliedSpec: "4cb74184589",
		},
	}
	canary4sum := canaryChecksum(canary4)

	require.Equal(t, canary1sum, canaryChecksum(canary1))
	require.NotEqual(t, canary1sum, canary2sum)
	require.NotEqual(t, canary2sum, canary3sum)
	require.NotEqual(t, canary3sum, canary1sum)
	require.NotEqual(t, canary4sum, canary1sum)
}
