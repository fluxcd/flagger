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
		return &NGrinderTask{
			TaskBase{canary, logger},
			baseUrl, cloneId, username, string(passwdDecoded), -1,
		}, nil
	})
}

type NGrinderTask struct {
	TaskBase
	baseUrl  *url.URL
	cloneId  int
	username string
	passwd   string
	testId   int
}

func (task *NGrinderTask) Hash() string {
	return hash(task.canary + task.CloneAndStartEndpoint().String())
}

func (task *NGrinderTask) CloneAndStartEndpoint() *url.URL {
	path, _ := url.Parse(fmt.Sprintf("perftest/api/%d/clone_and_start", task.cloneId))
	return task.baseUrl.ResolveReference(path)
}
func (task *NGrinderTask) StopEndpoint() *url.URL {
	path, _ := url.Parse(fmt.Sprintf("perftest/api/%d?action=stop", task.testId))
	return task.baseUrl.ResolveReference(path)
}

func (task *NGrinderTask) Run(ctx context.Context) bool {
	url := task.CloneAndStartEndpoint().String()
	result, err := task.request("POST", url, ctx)
	if err != nil {
		task.logger.With("canary", task.canary).Errorf("failed to clone and start ngrinder test %s: %s", url, err.Error())
		return false
	}
	id := result["id"]
	testId, ok := id.(int)
	if !ok {
		return false
	} else {
		task.testId = testId
		return task.PollStatus(ctx)
	}
}

func (task *NGrinderTask) String() string {
	return task.canary + task.CloneAndStartEndpoint().String()
}

func (task *NGrinderTask) PollStatus(ctx context.Context) bool {
	// wait until ngrinder test completed or timedout
	tickChan := time.NewTicker(time.Second * 15).C
	for {
		select {
		case <-tickChan:

		case <-ctx.Done():
			return false
		}
	}
}

func (task *NGrinderTask) request(method, url string, ctx context.Context) (map[string]interface{}, error) {
	req, _ := http.NewRequest(method, url, nil)
	req.SetBasicAuth(task.username, task.passwd)
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	res := make(map[string]interface{})
	json.Unmarshal(respBytes, res)
	err = nil
	if success, ok := res["success"]; ok && success == false {
		err = errors.New(res["message"].(string))
	}
	return res, err
}
