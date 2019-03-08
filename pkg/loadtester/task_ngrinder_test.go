package loadtester

import (
	"context"
	"github.com/stefanprodan/flagger/pkg/logging"
	"testing"
	"time"
)

func TestTaskNGrinder(t *testing.T) {
	logger, _ := logging.NewLoggerWithEncoding("debug", "console")
	canary := "podinfo.default"
	taskFactory, ok := GetTaskFactory(TaskTypeNGrinder)
	if !ok {
		t.Errorf("Failed to get ngrinder task factory")
	}
	t.Run("NormalRequest", func(t *testing.T) {
		task, err := taskFactory(map[string]string{
			"server":   "http://10.255.254.25:8080",
			"clone":    "960",
			"username": "admin",
			"passwd":   "YWRtaW4=",
		}, canary, logger)
		if err != nil {
			t.Fatalf("Failed to create ngrinder task: %s", err.Error())
			return
		}
		ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
		task.Run(ctx)
		<-ctx.Done()
	})
}
