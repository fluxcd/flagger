package loadtester

import (
	"context"
	"fmt"
	"os/exec"
)

const TaskTypeHelm = "helm"

type HelmTask struct {
	TaskBase
	command      string
	logCmdOutput bool
}

func (task *HelmTask) Hash() string {
	return hash(task.canary + task.command)
}

func (task *HelmTask) Run(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", "helm "+task.command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		task.logger.With("canary", task.canary).Errorf("command failed %s %v %s", task.command, err, out)
		return false, fmt.Errorf(" %v %v", err, out)
	} else {
		if task.logCmdOutput {
			fmt.Printf("%s\n", out)
		}
		task.logger.With("canary", task.canary).Infof("command finished %v", cmd.Args)
	}
	return true, nil
}

func (task *HelmTask) String() string {
	return task.command
}
