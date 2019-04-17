package loadtester

import (
	"context"
	"encoding/hex"
	"go.uber.org/zap"
	"hash/fnv"
	"sync"
)

// Modeling a loadtester task
type Task interface {
	Hash() string
	Run(ctx context.Context) bool
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
