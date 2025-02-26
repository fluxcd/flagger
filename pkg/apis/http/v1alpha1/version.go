package v1alpha1

import (
	"fmt"
	"runtime"

	"github.com/go-logr/logr"
)

var (
	version   = "main"
	gitCommit string
)

// Version returns the current git SHA of commit the binary was built from
func Version() string {
	return version
}

// GitCommit stores the current commit hash
func GitCommit() string {
	return gitCommit
}

func PrintComponentInfo(logger logr.Logger, component string) {
	logger.Info(fmt.Sprintf("%s Version: %s", component, Version()))
	logger.Info(fmt.Sprintf("%s Commit: %s", component, GitCommit()))
	logger.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	logger.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}
