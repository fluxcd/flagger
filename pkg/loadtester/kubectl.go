package loadtester

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const TaskTypeKubectl = "kubectl"

type KubectlTask struct {
	TaskBase
	command      string
	logCmdOutput bool
}

func (task *KubectlTask) Hash() string {
	return hash(task.canary + task.command)
}

func (task *KubectlTask) Run(ctx context.Context) (*TaskRunResult, error) {
	kubectlCmd := fmt.Sprintf("%s %s", TaskTypeKubectl, task.command)
	task.logger.With("canary", task.canary).Infof("running command %v", kubectlCmd)

	cmd := exec.CommandContext(ctx, TaskTypeKubectl, strings.Fields(task.command)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		task.logger.With("canary", task.canary).Errorf("command failed %s %v %s", task.command, err, out)
		return &TaskRunResult{false, out}, fmt.Errorf("command %s failed: %s: %w", task.command, out, err)
	} else {
		if task.logCmdOutput {
			task.logger.With("canary", task.canary).Info(string(out))
		}
		task.logger.With("canary", task.canary).Infof("command finished %v", kubectlCmd)
	}
	return &TaskRunResult{true, out}, nil
}

func (task *KubectlTask) String() string {
	return task.command
}
