package loadtester

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// TaskTypeConcord represents the concord type as string
const TaskTypeConcord = "concord"

// default process values
const concordBasePath = "/api/v1/process"
const defaultPollInterval = 5
const defaultPollTimeout = 30

// concord statuses
const concordStatusSuccess = "FINISHED"
const concordStatusFailed = "FAILED"

// ConcordTask represents a concord task
type ConcordTask struct {
	TaskBase
	Command      string
	Org          string
	Project      string
	Repo         string
	Entrypoint   string
	APIKeyPath   string
	Endpoint     string
	PollInterval time.Duration
	PollTimeout  time.Duration
	BaseURL      *url.URL
	httpClient   *http.Client
}

// NewConcordTask instantiates a new Concord Task
func NewConcordTask(metadata map[string]string, canary string, logger *zap.SugaredLogger) (*ConcordTask, error) {
	var pollIntervalInt, pollTimeoutInt int

	if _, found := metadata["server"]; !found {
		return nil, errors.New("`server` is required with type concord")
	}
	pURL, err := url.Parse(metadata["server"])
	if err != nil {
		return nil, errors.New("failed to create base URL from metadata `concord_url`")
	}
	if _, found := metadata["org"]; !found {
		return nil, errors.New("`org` is required with type concord")
	}
	if _, found := metadata["project"]; !found {
		return nil, errors.New("`project` is required with type concord")
	}
	if _, found := metadata["repo"]; found == false {
		return nil, errors.New("`repo` is required with type concord")
	}
	if _, found := metadata["entrypoint"]; found == false {
		return nil, errors.New("`entrypoint` is required with type concord")
	}
	if _, found := metadata["apiKeyPath"]; found == false {
		return nil, errors.New("`apiKeyPath` is required with type concord")
	}
	_, err = os.Stat(metadata["apiKeyPath"])
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("`apiKeyPath` file doesn't exist %s", metadata["apiKeyPath"])
	}
	if _, found := metadata["endpoint"]; found == false {
		return nil, errors.New("`endpoint` is required with type concord")
	}
	if _, found := metadata["pollInterval"]; found == false {
		pollIntervalInt = defaultPollInterval
	} else {
		pollIntervalInt, err = strconv.Atoi(metadata["pollInterval"])
		if err != nil {
			return nil, errors.New("unable to convert `pollInterval` to int")
		}
	}
	if _, found := metadata["pollTimeout"]; found == false {
		pollTimeoutInt = defaultPollTimeout
	} else {
		pollTimeoutInt, err = strconv.Atoi(metadata["pollTimeout"])
		if err != nil {
			return nil, errors.New("unable to convert `pollTimeout` to int")
		}
	}

	return &ConcordTask{
		TaskBase: TaskBase{
			logger: logger,
		},
		BaseURL:      pURL,
		Org:          metadata["org"],
		Project:      metadata["project"],
		Repo:         metadata["repo"],
		Entrypoint:   metadata["entrypoint"],
		APIKeyPath:   metadata["apiKeyPath"],
		Endpoint:     metadata["endpoint"],
		PollInterval: time.Duration(pollIntervalInt) * time.Second,
		PollTimeout:  time.Duration(pollTimeoutInt) * time.Second,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (task *ConcordTask) Hash() string {
	return hash(task.canary + task.Org + task.Project + task.Repo + task.Entrypoint)
}

func (task *ConcordTask) String() string {
	return fmt.Sprintf("%s %s %s %s", task.Org, task.Project, task.Repo, task.Entrypoint)
}

func (task *ConcordTask) Run(ctx context.Context) (*TaskRunResult, error) {
	instance, err := task.startProcess()
	if err != nil {
		task.logger.Errorf("failed to start process: %s", err.Error())
		return &TaskRunResult{false, nil}, err
	}

	ok, err := task.checkStatus(ctx, instance, task.PollInterval)
	return &TaskRunResult{ok, nil}, err
}

type concordProcess struct {
	InstanceID       string   `json:"instanceId,omitempty"`
	ParentInstanceID string   `json:"parentInstanceID,omitempty"`
	ProjectName      string   `json:"projectName,omitempty"`
	CreatedAt        string   `json:"createdAt,omitempty"`
	Initiator        string   `json:"initiator,omitempty"`
	LastUpdatedAt    string   `json:"lastUpdatedAt,omitempty"`
	Status           string   `json:"status,omitempty"`
	ChildrenIds      []string `json:"childrenIds,omitempty"`
	OK               bool     `json:"ok,omitempty"`
}

func (task *ConcordTask) newRequest(method, path string, contentType string, body io.Reader) (*http.Request, error) {
	rel := &url.URL{Path: path}
	u := task.BaseURL.ResolveReference(rel)
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	apiKey := ""
	dat, err := ioutil.ReadFile(task.APIKeyPath)
	if err != nil {
		return req, err
	}
	apiKey = string(dat)

	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}

	return req, nil
}

func (task *ConcordTask) do(req *http.Request, v interface{}) (*http.Response, error) {
	task.logger.With("canary", task.canary).Infof("calling endpoint %s", req.URL.String())
	resp, err := task.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(v)
	return resp, err
}

func (task *ConcordTask) startProcess() (string, error) {
	requestBody := new(bytes.Buffer)
	writer := multipart.NewWriter(requestBody)
	_ = writer.WriteField("org", task.Org)
	_ = writer.WriteField("project", task.Project)
	_ = writer.WriteField("repo", task.Repo)
	_ = writer.WriteField("entryPoint", task.Entrypoint)
	_ = writer.WriteField("arguments.endpoint", task.Endpoint)

	err := writer.Close()
	if err != nil {
		return "", nil
	}

	req, err := task.newRequest(http.MethodPost, concordBasePath, writer.FormDataContentType(), requestBody)
	if err != nil {
		return "", err
	}

	process := concordProcess{}
	_, err = task.do(req, &process)
	if err != nil {
		return "", err
	}

	task.logger.With("canary", task.canary).Infof("created process id [%s] OK status is %t", process.InstanceID, process.OK)
	return process.InstanceID, nil
}

func (task *ConcordTask) checkStatus(ctx context.Context, instanceID string, interval time.Duration) (bool, error) {
	tickChan := time.NewTicker(interval).C
	for {
		select {
		case <-tickChan:
			req, err := task.newRequest(http.MethodGet, fmt.Sprintf("%s/%s", concordBasePath, instanceID), "", nil)
			if err != nil {
				return false, fmt.Errorf("failed to generate request: %s", err)
			}

			process := concordProcess{}
			_, err = task.do(req, &process)
			if err != nil {
				return false, fmt.Errorf("failed checking status: %s", err)
			}
			task.logger.With("canary", task.canary).Infof("process id [%s] current status is %s", process.InstanceID, process.Status)

			if process.Status == concordStatusSuccess {
				return true, nil
			}
			if process.Status == concordStatusFailed {
				return false, fmt.Errorf("concord instanceID: %s failed", instanceID)
			}
		case <-time.After(task.PollTimeout):
			return false, fmt.Errorf("concord process timed out, after %d seconds", int64(task.PollTimeout/time.Second))
		case <-ctx.Done():
			return false, errors.New("context timedout")
		}

	}
}
