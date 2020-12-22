package loadtester

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"

	"github.com/stretchr/testify/assert"
)

func TestServer_HandleHealthz(t *testing.T) {
	req, _ := http.NewRequest("GET", "/heathz", nil)
	resp := httptest.NewRecorder()
	HandleHealthz(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "OK", resp.Body.String())
}

func TestServer_HandleNewBashTaskCmdExitZero(t *testing.T) {
	mocks := newServerFixture()
	resp := mocks.resp
	req := newJsonRequest("POST", "/", &flaggerv1.CanaryWebhookPayload{
		Metadata: map[string]string{
			"type": TaskTypeBash,
			"cmd":  "echo some-output-not-to-be-returned",
		},
	})
	HandleNewTask(mocks.logger, mocks.taskRunner)(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Empty(t, resp.Body.String())
}

func TestServer_HandleNewBashTaskCmdExitZeroReturnCmdOutput(t *testing.T) {
	mocks := newServerFixture()
	resp := mocks.resp
	req := newJsonRequest("POST", "/", &flaggerv1.CanaryWebhookPayload{
		Metadata: map[string]string{
			"type":            TaskTypeBash,
			"cmd":             "echo some-output-to-be-returned",
			"returnCmdOutput": "true",
		},
	})
	HandleNewTask(mocks.logger, mocks.taskRunner)(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "some-output-to-be-returned\n", resp.Body.String())
}

func TestServer_HandleNewBashTaskCmdExitNonZero(t *testing.T) {
	mocks := newServerFixture()
	resp := mocks.resp
	req := newJsonRequest("POST", "/", &flaggerv1.CanaryWebhookPayload{
		Metadata: map[string]string{
			"type": TaskTypeBash,
			"cmd":  "false",
		},
	})

	HandleNewTask(mocks.logger, mocks.taskRunner)(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Equal(t, "command false failed: : exit status 1", resp.Body.String())
}

func newJsonRequest(method string, url string, v interface{}) *http.Request {
	payload, _ := json.Marshal(v)
	req, _ := http.NewRequest(method, url, bytes.NewReader(payload))
	return req
}
