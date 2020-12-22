package loadtester

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/weaveworks/flagger/pkg/logger"
)

func TestTaskRunner_Start(t *testing.T) {
	stop := make(chan struct{})
	logger, _ := logger.NewLogger("debug")
	tr := NewTaskRunner(logger, time.Hour)

	go tr.Start(10*time.Millisecond, stop)

	taskFactory, _ := GetTaskFactory(TaskTypeShell)
	task1, _ := taskFactory(map[string]string{"type": "cmd", "cmd": "sleep 0.6"}, "podinfo.default", logger)
	task2, _ := taskFactory(map[string]string{"cmd": "sleep 0.7", "logCmdOutput": "true"}, "podinfo.default", logger)

	tr.Add(task1)
	tr.Add(task2)

	time.Sleep(100 * time.Millisecond)

	tr.Add(task1)
	tr.Add(task2)

	time.Sleep(100 * time.Millisecond)

	tr.Add(task1)
	tr.Add(task2)

	assert.Equal(t, uint64(2), tr.GetTotalExecs())

	time.Sleep(time.Second)

	tr.Add(task1)
	tr.Add(task2)

	time.Sleep(time.Second)
	assert.Equal(t, uint64(4), tr.GetTotalExecs())
}
