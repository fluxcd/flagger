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
	"encoding/hex"
	"hash/fnv"
	"sync"

	"go.uber.org/zap"
)

// Modeling a loadtester task
type Task interface {
	Hash() string
	Run(ctx context.Context) *TaskRunResult
	String() string
	Canary() string
}

type TaskBase struct {
	canary string
	logger *zap.SugaredLogger
}

func (task *TaskBase) Canary() string {
	return task.canary
}

func hash(str string) string {
	fnvHash := fnv.New32()
	fnvBytes := fnvHash.Sum([]byte(str))
	return hex.EncodeToString(fnvBytes[:])
}

var taskFactories = new(sync.Map)

type TaskFactory = func(metadata map[string]string, canary string, logger *zap.SugaredLogger) (Task, error)

func GetTaskFactory(typ string) (TaskFactory, bool) {
	factory, ok := taskFactories.Load(typ)
	return factory.(TaskFactory), ok
}

type TaskRunResult struct {
	ok  bool
	out []byte
}
