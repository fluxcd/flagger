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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fluxcd/flagger/pkg/logger"
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
