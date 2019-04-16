package loadtester

import (
	"context"
	"fmt"
	"github.com/weaveworks/flagger/pkg/logger"
	"gopkg.in/h2non/gock.v1"
	"testing"
	"time"
)

func TestTaskNGrinder(t *testing.T) {
	server := "http://ngrinder:8080"
	cloneId := "960"
	logger, _ := logger.NewLoggerWithEncoding("debug", "console")
	canary := "podinfo.default"
	taskFactory, ok := GetTaskFactory(TaskTypeNGrinder)
	if !ok {
		t.Errorf("Failed to get ngrinder task factory")
	}

	defer gock.Off()
	gock.New(server).Post(fmt.Sprintf("perftest/api/%s/clone_and_start", cloneId)).
		Reply(200).BodyString(`{"status": "READY","id": 961}`)
	gock.New(server).Get("perftest/api/961/status").Reply(200).
		BodyString(`{"status": [{"status_id": "FINISHED"}]}`)
	gock.New(server).Put("perftest/api/961").MatchParam("action", "stop").Reply(200).
		BodyString(`{"success": true}`)

	t.Run("NormalRequest", func(t *testing.T) {
		task, err := taskFactory(map[string]string{
			"server":       server,
			"clone":        cloneId,
			"username":     "admin",
			"passwd":       "YWRtaW4=",
			"pollInterval": "1s",
		}, canary, logger)
		if err != nil {
			t.Fatalf("Failed to create ngrinder task: %s", err.Error())
			return
		}
		ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
		task.Run(ctx)
		<-ctx.Done()
	})
}
