package loadtester

import (
	"net/http/httptest"
	"time"

	"github.com/weaveworks/flagger/pkg/logger"
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
