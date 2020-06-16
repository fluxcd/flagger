package loadtester

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/assert"
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

	slurp, err := ioutil.ReadAll(part)
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
