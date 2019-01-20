package loadtester

import (
	"github.com/stefanprodan/flagger/pkg/logging"
	"testing"
	"time"
)

func TestTaskRunner_Start(t *testing.T) {
	stop := make(chan struct{})
	logger, _ := logging.NewLogger("debug")
	tr := NewTaskRunner(logger, time.Hour)

	go tr.Start(10*time.Millisecond, stop)

	task1 := Task{
		Canary:  "podinfo.default",
		Command: "sleep 0.6",
	}
	task2 := Task{
		Canary:  "podinfo.default",
		Command: "sleep 0.7",
	}

	tr.Add(task1)
	tr.Add(task2)

	time.Sleep(100 * time.Millisecond)

	tr.Add(task1)
	tr.Add(task2)

	time.Sleep(100 * time.Millisecond)

	tr.Add(task1)
	tr.Add(task2)

	if tr.GetTotalExecs() != 2 {
		t.Errorf("Got total executed commands %v wanted %v", tr.GetTotalExecs(), 2)
	}

	time.Sleep(time.Second)

	tr.Add(task1)
	tr.Add(task2)

	time.Sleep(time.Second)

	if tr.GetTotalExecs() != 4 {
		t.Errorf("Got total executed commands %v wanted %v", tr.GetTotalExecs(), 4)
	}
}
