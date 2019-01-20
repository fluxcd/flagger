package loadtester

import (
	"context"
	"encoding/hex"
	"go.uber.org/zap"
	"hash/fnv"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type TaskRunner struct {
	logger       *zap.SugaredLogger
	timeout      time.Duration
	todoTasks    *sync.Map
	runningTasks *sync.Map
	totalExecs   uint64
}

type Task struct {
	Canary  string
	Command string
}

func (t Task) Hash() string {
	fnvHash := fnv.New32()
	fnvBytes := fnvHash.Sum([]byte(t.Canary + t.Command))
	return hex.EncodeToString(fnvBytes[:])
}

func NewTaskRunner(logger *zap.SugaredLogger, timeout time.Duration) *TaskRunner {
	return &TaskRunner{
		logger:       logger,
		todoTasks:    new(sync.Map),
		runningTasks: new(sync.Map),
		timeout:      timeout,
	}
}

func (tr *TaskRunner) Add(task Task) {
	tr.todoTasks.Store(task.Hash(), task)
}

func (tr *TaskRunner) GetTotalExecs() uint64 {
	return atomic.LoadUint64(&tr.totalExecs)
}

func (tr *TaskRunner) runAll() {
	tr.todoTasks.Range(func(key interface{}, value interface{}) bool {
		task := value.(Task)
		go func(t Task) {
			// remove task from the to do list
			tr.todoTasks.Delete(t.Hash())

			// check if task is already running, if not run the task's command
			if _, exists := tr.runningTasks.Load(t.Hash()); !exists {
				// save the task in the running list
				tr.runningTasks.Store(t.Hash(), t)

				// create timeout context
				ctx, cancel := context.WithTimeout(context.Background(), tr.timeout)
				defer cancel()

				// increment the total exec counter
				atomic.AddUint64(&tr.totalExecs, 1)

				tr.logger.With("canary", t.Canary).Infof("command starting %s", t.Command)
				cmd := exec.CommandContext(ctx, "sh", "-c", t.Command)

				// execute task
				out, err := cmd.CombinedOutput()
				if err != nil {
					tr.logger.With("canary", t.Canary).Errorf("command failed %s %v %s", t.Command, err, out)
				} else {
					tr.logger.With("canary", t.Canary).Infof("command finished %s", t.Command)
				}

				// remove task from the running list
				tr.runningTasks.Delete(t.Hash())
			} else {
				tr.logger.With("canary", t.Canary).Infof("command skipped %s is already running", t.Command)
			}
		}(task)
		return true
	})
}

func (tr *TaskRunner) Start(interval time.Duration, stopCh <-chan struct{}) {
	tickChan := time.NewTicker(interval).C
	for {
		select {
		case <-tickChan:
			tr.runAll()
		case <-stopCh:
			tr.logger.Info("shutting down the task runner")
			return
		}
	}
}
