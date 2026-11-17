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

package loadtester

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewConcordTask_Successful(t *testing.T) {
	metadata := map[string]string{
		"server":     "example.org",
		"project":    "my project",
		"repo":       "my repo",
		"org":        "my org",
		"entrypoint": "my-entrypoint",
		"endpoint":   "example.org/path/to/thing",
		"apiKeyPath": "/",
	}
	task, err := NewConcordTask(metadata, "canary", zap.NewExample().Sugar())

	assert.IsType(t, &ConcordTask{}, task, "Expected to get a well-formed concord task out")
	assert.Equal(t, nil, err)
}

func TestNewConcordTask_InitializationWithoutAdequateArgs(t *testing.T) {
	metadata := map[string]string{
		"test": "foo",
	}
	_, err := NewConcordTask(metadata, "canary", zap.NewExample().Sugar())

	assert.Error(t, err, "is required with type concord")
}

func TestNewConcordTask_AdditionalArguments(t *testing.T) {
	metadata := map[string]string{
		"server":         "example.org",
		"project":        "my project",
		"repo":           "my repo",
		"org":            "my org",
		"entrypoint":     "my-entrypoint",
		"endpoint":       "example.org/path/to/thing",
		"apiKeyPath":     "/",
		"arguments.test": "works",
	}
	task, err := NewConcordTask(metadata, "canary", zap.NewExample().Sugar())

	assert.IsType(t, &ConcordTask{}, task, "Expected to get a well-formed concord task out")
	assert.Equal(t, nil, err)
	assert.Equal(t, task.Arguments["test"], "works")
}

func TestNewConcordTask_DontOverrideEndpoint(t *testing.T) {
	metadata := map[string]string{
		"server":             "example.org",
		"project":            "my project",
		"repo":               "my repo",
		"org":                "my org",
		"entrypoint":         "my-entrypoint",
		"endpoint":           "example.org/path/to/thing",
		"apiKeyPath":         "/",
		"arguments.endpoint": "works",
	}
	_, err := NewConcordTask(metadata, "canary", zap.NewExample().Sugar())

	assert.Error(t, err, "You cannot override Endpoint through arguments")

}

func assertNextPartHasKeyAndValue(t *testing.T, r *multipart.Reader, key string, value string) {
	part, err := r.NextPart()
	if err != nil {
		t.Fatalf("Part failed: %v", err)
	}
	// assert.Equal(t, 1, part)

	slurp, err := io.ReadAll(part)
	if err != nil {
		fmt.Printf("Part: %+v", part)
		t.Fatalf("Couldn't read part: %v", err)
	}

	assert.Equal(t, part.FormName(), key)
	assert.Equal(t, string(slurp), value)

}

func TestConcordTask_BuildingDefaultFields(t *testing.T) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	task := &ConcordTask{
		Org:        "my org",
		Project:    "my project",
		Repo:       "my repo",
		Entrypoint: "my entrypoint",
		Endpoint:   "example.org",
	}

	task.buildFields(w)

	err := w.Close()
	if err != nil {
		t.Fatalf("Couldn't close writer: %v", err)
	}

	r := multipart.NewReader(&b, w.Boundary())
	assertNextPartHasKeyAndValue(t, r, "org", "my org")
	assertNextPartHasKeyAndValue(t, r, "project", "my project")
	assertNextPartHasKeyAndValue(t, r, "repo", "my repo")
	assertNextPartHasKeyAndValue(t, r, "entryPoint", "my entrypoint")
	assertNextPartHasKeyAndValue(t, r, "arguments.endpoint", "example.org")

	part, _ := r.NextPart()
	if part != nil {
		t.Errorf("Didn't expect additional parts, but got %v", part)
	}
}

func TestConcordTask_AdditionalArguments(t *testing.T) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	task := &ConcordTask{
		Org:        "my org",
		Project:    "my project",
		Repo:       "my repo",
		Entrypoint: "my entrypoint",
		Endpoint:   "example.org",
		Arguments: map[string]string{
			"test": "thing",
		},
	}

	task.buildFields(w)

	err := w.Close()
	if err != nil {
		t.Fatalf("Couldn't close writer: %v", err)
	}

	r := multipart.NewReader(&b, w.Boundary())
	assertNextPartHasKeyAndValue(t, r, "org", "my org")
	assertNextPartHasKeyAndValue(t, r, "project", "my project")
	assertNextPartHasKeyAndValue(t, r, "repo", "my repo")
	assertNextPartHasKeyAndValue(t, r, "entryPoint", "my entrypoint")
	assertNextPartHasKeyAndValue(t, r, "arguments.endpoint", "example.org")
	assertNextPartHasKeyAndValue(t, r, "arguments.test", "thing")

	part, _ := r.NextPart()
	if part != nil {
		t.Errorf("Didn't expect additional parts, but got %v", part)
	}
}

func TestConcordTask_PollTimeout(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			fmt.Fprint(w, `{"instanceId":"45beb7c7-6aa2-40e4-ba1d-488f78700ab7","ok":true}`)
		case http.MethodGet:
			polls.Add(1)
			fmt.Fprint(w, `{"instanceId":"45beb7c7-6aa2-40e4-ba1d-488f78700ab7","status":"RUNNING"}`)
		}
	}))
	defer server.Close()

	apiKeyPath := filepath.Join(t.TempDir(), "api-key")
	require.NoError(t, os.WriteFile(apiKeyPath, []byte("test-key"), 0600))
	task, err := NewConcordTask(map[string]string{
		"server":     server.URL,
		"org":        "test-org",
		"project":    "test-project",
		"repo":       "test-repo",
		"entrypoint": "default",
		"endpoint":   "http://podinfo-canary:9898",
		"apiKeyPath": apiKeyPath,
	}, "podinfo.test", zap.NewNop().Sugar())
	require.NoError(t, err)
	task.PollInterval = 10 * time.Millisecond
	task.PollTimeout = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := task.Run(ctx)
	assert.False(t, result.ok)
	require.ErrorContains(t, err, "concord process timed out")
	assert.NoError(t, ctx.Err(), "Polling should time out before the task context")
	assert.Positive(t, polls.Load(), "Expected status polling before timeout")
}
