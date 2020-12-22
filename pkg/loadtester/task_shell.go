package loadtester

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"go.uber.org/zap"
)

const TaskTypeShell = "cmd"

func init() {
	taskFactories.Store(TaskTypeShell, func(metadata map[string]string, canary string, logger *zap.SugaredLogger) (Task, error) {
		cmd, ok := metadata["cmd"]
		if !ok {
			return nil, errors.New("cmd not found in metadata")
		}
		logCmdOutput, _ := strconv.ParseBool(metadata["logCmdOutput"])
		return &CmdTask{TaskBase{canary, logger}, cmd, logCmdOutput}, nil
	})
}

type CmdTask struct {
	TaskBase
	command      string
	logCmdOutput bool
}

func (task *CmdTask) Hash() string {
	return hash(task.canary + task.command)
}

func (task *CmdTask) Run(ctx context.Context) *TaskRunResult {
	cmd := exec.CommandContext(ctx, "sh", "-c", task.command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		task.logger.With("canary", task.canary).Errorf("command failed %s %v %s", task.command, err, out)
	} else {
		if task.logCmdOutput {
			fmt.Printf("%s\n", out)
		}
		task.logger.With("canary", task.canary).Infof("command finished %s", task.command)
	}
	return &TaskRunResult{err == nil, out}
}

func (task *CmdTask) String() string {
	return task.command
}
