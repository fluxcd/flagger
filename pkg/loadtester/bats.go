package loadtester

import (
	"context"
	"fmt"
	"os/exec"
)

const TaskTypeBats = "bats"

type BatsTask struct {
	TaskBase
	command      string
	logCmdOutput bool
}

func (task *BatsTask) Hash() string {
	return hash(task.canary + task.command)
}

func (task *BatsTask) Run(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", task.command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		task.logger.With("canary", task.canary).Errorf("command failed %s %v %s", task.command, err, out)
		return false, fmt.Errorf(" %v %v", err, out)
	} else {
		if task.logCmdOutput {
			fmt.Printf("%s\n", out)
		}
		task.logger.With("canary", task.canary).Infof("command finished %s", task.command)
	}
	return true, nil
}

func (task *BatsTask) String() string {
	return task.command
}
