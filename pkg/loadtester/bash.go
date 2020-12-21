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
	"context"
	"fmt"
	"os/exec"
)

const TaskTypeBash = "bash"

type BashTask struct {
	TaskBase
	command      string
	logCmdOutput bool
}

func (task *BashTask) Hash() string {
	return hash(task.canary + task.command)
}

func (task *BashTask) Run(ctx context.Context) (*TaskRunResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", task.command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		task.logger.With("canary", task.canary).Errorf("command failed %s %v %s", task.command, err, out)
		return &TaskRunResult{false, out}, fmt.Errorf("command %s failed: %s: %w", task.command, out, err)
	} else {
		if task.logCmdOutput {
			fmt.Printf("%s\n", out)
		}
		task.logger.With("canary", task.canary).Infof("command finished %s", task.command)
	}
	return &TaskRunResult{true, out}, nil
}

func (task *BashTask) String() string {
	return task.command
}
