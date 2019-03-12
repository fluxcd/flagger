package loadtester

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const TaskTypeNGrinder = "ngrinder"

func init() {
	taskFactories.Store(TaskTypeNGrinder, func(metadata map[string]string, canary string, logger *zap.SugaredLogger) (Task, error) {
		server := metadata["server"]
		clone := metadata["clone"]
		username := metadata["username"]
		passwd := metadata["passwd"]
		pollInterval := metadata["pollInterval"]
		if server == "" || clone == "" || username == "" || passwd == "" {
			return nil, errors.New("server, clone, username and passwd are required metadata")
		}
		baseUrl, err := url.Parse(server)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("invalid url: %s", server))
		}
		cloneId, err := strconv.Atoi(clone)
		if err != nil {
			return nil, errors.New("metadata clone must be integer")
		}

		passwdDecoded, err := base64.StdEncoding.DecodeString(passwd)
		if err != nil {
			return nil, errors.New("metadata auth provided is invalid, base64 encoded username:password required")
		}
		interval, err := time.ParseDuration(pollInterval)
		if err != nil {
			interval = 1
		}

		return &NGrinderTask{
			TaskBase{canary, logger},
			baseUrl, cloneId, username, string(passwdDecoded), -1, interval,
		}, nil
	})
}

type NGrinderTask struct {
	TaskBase
	// base url of ngrinder server, e.g. http://ngrinder:8080
	baseUrl *url.URL
	// template test to clone from
	cloneId int
	// http basic auth
	username string
	passwd   string
	// current ngrinder test id
	testId int
	// task status polling interval
	pollInterval time.Duration
}

func (task *NGrinderTask) Hash() string {
	return hash(task.canary + string(task.cloneId))
}

// nGrinder REST endpoints
func (task *NGrinderTask) CloneAndStartEndpoint() *url.URL {
	path, _ := url.Parse(fmt.Sprintf("perftest/api/%d/clone_and_start", task.cloneId))
	return task.baseUrl.ResolveReference(path)
}
func (task *NGrinderTask) StatusEndpoint() *url.URL {
	path, _ := url.Parse(fmt.Sprintf("perftest/api/%d/status", task.testId))
	return task.baseUrl.ResolveReference(path)
}
func (task *NGrinderTask) StopEndpoint() *url.URL {
	path, _ := url.Parse(fmt.Sprintf("perftest/api/%d?action=stop", task.testId))
	return task.baseUrl.ResolveReference(path)
}

// initiate a clone_and_start request and get new test id from response
func (task *NGrinderTask) Run(ctx context.Context) bool {
	url := task.CloneAndStartEndpoint().String()
	result, err := task.request("POST", url, ctx)
	if err != nil {
		task.logger.With("canary", task.canary).Errorf("failed to clone and start ngrinder test %s: %s", url, err.Error())
		return false
	}
	id := result["id"]
	task.testId = int(id.(float64))
	return task.PollStatus(ctx)
}

func (task *NGrinderTask) String() string {
	return task.canary + task.CloneAndStartEndpoint().String()
}

// polling execution status of the new test and check if finished
func (task *NGrinderTask) PollStatus(ctx context.Context) bool {
	// wait until ngrinder test finished/canceled or timedout
	tickChan := time.NewTicker(time.Second * task.pollInterval).C
	for {
		select {
		case <-tickChan:
			result, err := task.request("GET", task.StatusEndpoint().String(), ctx)
			if err == nil {
				statusArray, ok := result["status"].([]interface{})
				if ok && len(statusArray) > 0 {
					status := statusArray[0].(map[string]interface{})
					statusId := status["status_id"]
					task.logger.Debugf("status of ngrinder task %d is %s", task.testId, statusId)
					if statusId == "FINISHED" {
						return true
					} else if statusId == "STOP_BY_ERROR" || statusId == "CANCELED" || statusId == "UNKNOWN" {
						return false
					}
				}
			}
		case <-ctx.Done():
			task.logger.Warnf("context timedout, top ngrinder task %d forcibly", task.testId)
			task.request("PUT", task.StopEndpoint().String(), nil)
			return false
		}
	}
}

// send request, handle error, and eavl response json
func (task *NGrinderTask) request(method, url string, ctx context.Context) (map[string]interface{}, error) {
	task.logger.Debugf("send %s request to %s", method, url)
	req, _ := http.NewRequest(method, url, nil)
	req.SetBasicAuth(task.username, task.passwd)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		task.logger.Errorf("bad request: %s", err.Error())
		return nil, err
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	res := make(map[string]interface{})
	err = json.Unmarshal(respBytes, &res)
	if err != nil {
		task.logger.Errorf("bad response, %s ,json expected:\n %s", err.Error(), string(respBytes))
	} else if success, ok := res["success"]; ok && success == false {
		err = errors.New(res["message"].(string))
	}
	return res, err
}
