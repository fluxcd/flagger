package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhooks(t *testing.T) {
	notReady := flaggerv1.CanaryWebhookPayload{
		Metadata: map[string]string{
			"eventMessage": "podinfo-primary.default not ready: waiting for rollout to finish: 0 out of 1 new replicas have been updated",
		},
	}
	metricsInitialized := flaggerv1.CanaryWebhookPayload{
		Metadata: map[string]string{
			"eventMessage": "all the metrics providers are available!",
		},
	}

	tc := []struct {
		name           string
		expectedEvents map[string][]flaggerv1.CanaryWebhookPayload
		test           func(t *testing.T, mock fixture)
	}{
		{
			name: "skip-analysis",
			expectedEvents: map[string][]flaggerv1.CanaryWebhookPayload{
				"event": {
					metricsInitialized,
					notReady,
					metricsInitialized,
					{Phase: flaggerv1.CanaryPhaseInitialized, Metadata: map[string]string{"eventMessage": "Initialization done! podinfo.default"}},
					{Phase: flaggerv1.CanaryPhaseInitialized, Metadata: map[string]string{"eventMessage": "Confirm-rollout check confirm-rollout-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "New revision detected! Scaling up podinfo.default"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Copying podinfo.default template spec to podinfo-primary.default"}},
					{Phase: flaggerv1.CanaryPhaseSucceeded, Metadata: map[string]string{"eventMessage": "Promotion completed! Canary analysis was skipped for podinfo.default"}},
				},
				"confirm-rollout": {
					{Phase: flaggerv1.CanaryPhaseInitialized, Metadata: map[string]string{"eventMessage": ""}},
				},
			},
			test: func(t *testing.T, mocks fixture) {
				mocks.ctrl.advanceCanary("podinfo", "default")
				mocks.makePrimaryReady(t)
				mocks.ctrl.advanceCanary("podinfo", "default")

				// enable skip
				cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(t.Context(), "podinfo", metav1.GetOptions{})
				require.NoError(t, err)
				cd.Spec.SkipAnalysis = true
				_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(t.Context(), cd, metav1.UpdateOptions{})
				require.NoError(t, err)

				// update
				dep2 := newDeploymentTestDeploymentV2()
				_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(t.Context(), dep2, metav1.UpdateOptions{})
				require.NoError(t, err)

				// detect changes
				mocks.ctrl.advanceCanary("podinfo", "default")
				mocks.makeCanaryReady(t)

				// advance
				mocks.ctrl.advanceCanary("podinfo", "default")
			},
		},
		{
			name: "canary",
			expectedEvents: map[string][]flaggerv1.CanaryWebhookPayload{
				"event": {
					metricsInitialized,
					notReady,
					metricsInitialized,
					{Phase: flaggerv1.CanaryPhaseInitialized, Metadata: map[string]string{"eventMessage": "Initialization done! podinfo.default"}},
					{Phase: flaggerv1.CanaryPhaseInitialized, Metadata: map[string]string{"eventMessage": "Confirm-rollout check confirm-rollout-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "New revision detected! Scaling up podinfo.default"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Starting canary analysis for podinfo.default"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Pre-rollout check pre-rollout-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Confirm-traffic-increase check confirm-traffic-increase-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Advance podinfo.default canary weight 100"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Confirm-traffic-increase check confirm-traffic-increase-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Confirm-promotion check confirm-promotion-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": "Copying podinfo.default template spec to podinfo-primary.default"}},
					{Phase: flaggerv1.CanaryPhasePromoting, Metadata: map[string]string{"eventMessage": "Advance podinfo.default primary weight 50"}},
					{Phase: flaggerv1.CanaryPhasePromoting, Metadata: map[string]string{"eventMessage": "Advance podinfo.default primary weight 100"}},
					{Phase: flaggerv1.CanaryPhaseSucceeded, Metadata: map[string]string{"eventMessage": "Post-rollout check post-rollout-webhook passed"}},
					{Phase: flaggerv1.CanaryPhaseSucceeded, Metadata: map[string]string{"eventMessage": "Promotion completed! Scaling down podinfo.default"}},
				},
				"confirm-rollout": {
					{Phase: flaggerv1.CanaryPhaseInitialized, Metadata: map[string]string{"eventMessage": ""}},
				},
				"confirm-promotion": {
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": ""}},
				},
				"confirm-traffic-increase": {
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": ""}},
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": ""}},
				},
				"pre-rollout": {
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": ""}},
				},
				"post-rollout": {
					{Phase: flaggerv1.CanaryPhaseSucceeded, Metadata: map[string]string{"eventMessage": ""}},
				},
				"rollout": {
					{Phase: flaggerv1.CanaryPhaseProgressing, Metadata: map[string]string{"eventMessage": ""}},
				},
			},
			test: func(t *testing.T, mocks fixture) {
				mocks.canary.Spec.Analysis.Interval = "1m"
				mocks.canary.Spec.Analysis.StepWeight = 100
				mocks.canary.Spec.Analysis.StepWeightPromotion = 50
				_, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(t.Context(), mocks.canary, metav1.UpdateOptions{})
				require.NoError(t, err)

				// initializing
				mocks.ctrl.advanceCanary("podinfo", "default")

				// make primary ready
				mocks.makePrimaryReady(t)

				// initialized
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseInitialized))

				// update
				dep2 := newDeploymentTestDeploymentV2()
				_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(t.Context(), dep2, metav1.UpdateOptions{})
				require.NoError(t, err)

				// detect changes
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))
				mocks.makeCanaryReady(t)

				// progressing
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))

				// start promotion
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhasePromoting))

				// end promotion
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhasePromoting))

				// finalising
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseFinalising))

				// succeeded
				mocks.ctrl.advanceCanary("podinfo", "default")
				require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseSucceeded))
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			events := map[string][]flaggerv1.CanaryWebhookPayload{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				var payload flaggerv1.CanaryWebhookPayload
				err := json.NewDecoder(r.Body).Decode(&payload)
				require.NoError(t, err)

				// only record the event attributes we're comparing
				eventType := r.URL.Path[1:]
				events[eventType] = append(events[eventType], flaggerv1.CanaryWebhookPayload{
					Phase: payload.Phase,
					Metadata: map[string]string{
						"eventMessage": payload.Metadata["eventMessage"],
					},
				})

				w.WriteHeader(http.StatusOK)
			}))

			defer server.Close()

			c := newDeploymentTestCanary()
			c.Spec.Analysis.Webhooks = []flaggerv1.CanaryWebhook{
				{
					Type: flaggerv1.EventHook,
					Name: "event-webhook",
					URL:  server.URL + "/event",
				},
				{
					Type: flaggerv1.PreRolloutHook,
					Name: "pre-rollout-webhook",
					URL:  server.URL + "/pre-rollout",
				},
				{
					Type: flaggerv1.RolloutHook,
					Name: "rollout-webhook",
					URL:  server.URL + "/rollout",
				},
				{
					Type: flaggerv1.ConfirmPromotionHook,
					Name: "confirm-promotion-webhook",
					URL:  server.URL + "/confirm-promotion",
				},
				{
					Type: flaggerv1.ConfirmRolloutHook,
					Name: "confirm-rollout-webhook",
					URL:  server.URL + "/confirm-rollout",
				},
				{
					Type: flaggerv1.PostRolloutHook,
					Name: "post-rollout-webhook",
					URL:  server.URL + "/post-rollout",
				},
				{
					Type: flaggerv1.ConfirmTrafficIncreaseHook,
					Name: "confirm-traffic-increase-webhook",
					URL:  server.URL + "/confirm-traffic-increase",
				},
			}

			mocks := newDeploymentFixture(c)
			tt.test(t, mocks)

			assert.Equal(t, tt.expectedEvents, events)
		})
	}
}
