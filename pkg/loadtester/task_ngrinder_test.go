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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gock "gopkg.in/h2non/gock.v1"

	"github.com/fluxcd/flagger/pkg/logger"
)

func TestTaskNGrinder(t *testing.T) {
	server := "http://ngrinder:8080"
	cloneId := "960"
	logger, _ := logger.NewLoggerWithEncoding("debug", "console")
	canary := "podinfo.default"
	taskFactory, ok := GetTaskFactory(TaskTypeNGrinder)
	assert.True(t, ok, "Failed to get ngrinder task factory")

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
		require.NoError(t, err, "Failed to create ngrinder task")
		ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
		task.Run(ctx)
		<-ctx.Done()
	})
}
