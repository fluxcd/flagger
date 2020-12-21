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
	"net/http/httptest"
	"time"

	"github.com/fluxcd/flagger/pkg/logger"
	"go.uber.org/zap"
)

type serverFixture struct {
	taskRunner *MockTaskRunner
	resp       *httptest.ResponseRecorder
	logger     *zap.SugaredLogger
}

func newServerFixture() serverFixture {
	taskRunner := &MockTaskRunner{}
	resp := httptest.NewRecorder()
	logger, _ := logger.NewLogger("info")

	return serverFixture{
		taskRunner: taskRunner,
		resp:       resp,
		logger:     logger,
	}
}

type MockTaskRunner struct {
}

func (m *MockTaskRunner) Add(task Task) {

}

func (m *MockTaskRunner) GetTotalExecs() uint64 {
	return 0
}

func (m *MockTaskRunner) Start(interval time.Duration, stopCh <-chan struct{}) {

}

func (m *MockTaskRunner) Timeout() time.Duration {
	return time.Hour
}
