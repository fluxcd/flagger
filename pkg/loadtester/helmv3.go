package loadtester

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const TaskTypeHelmv3 = "helmv3"

type HelmTaskv3 struct {
	TaskBase
	command      string
	logCmdOutput bool
}

func (task *HelmTaskv3) Hash() string {
	return hash(task.canary + task.command)
}

func (task *HelmTaskv3) Run(ctx context.Context) (*TaskRunResult, error) {
	helmCmd := fmt.Sprintf("%s %s", TaskTypeHelmv3, task.command)
	task.logger.With("canary", task.canary).Infof("running command %v", helmCmd)

	cmd := exec.CommandContext(ctx, TaskTypeHelmv3, strings.Fields(task.command)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		task.logger.With("canary", task.canary).Errorf("command failed %s %v %s", task.command, err, out)
		return &TaskRunResult{false, out}, fmt.Errorf("command %s failed: %s: %w", task.command, out, err)
	} else {
		if task.logCmdOutput {
			fmt.Printf("%s\n", out)
		}
		task.logger.With("canary", task.canary).Infof("command finished %v", helmCmd)
	}
	return &TaskRunResult{true, out}, nil
}

func (task *HelmTaskv3) String() string {
	return task.command
}
