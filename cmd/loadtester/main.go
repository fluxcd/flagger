package main

import (
	"flag"
	"github.com/knative/pkg/signals"
	"github.com/stefanprodan/flagger/pkg/loadtester"
	"github.com/stefanprodan/flagger/pkg/logging"
	"log"
	"time"
)

var VERSION = "0.0.2"
var (
	logLevel     string
	port         string
	timeout      time.Duration
	logCmdOutput bool
)

func init() {
	flag.StringVar(&logLevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&port, "port", "9090", "Port to listen on.")
	flag.DurationVar(&timeout, "timeout", time.Hour, "Command exec timeout.")
	flag.BoolVar(&logCmdOutput, "log-cmd-output", true, "Log command output to stderr")
}

func main() {
	flag.Parse()

	logger, err := logging.NewLogger(logLevel)
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}
	defer logger.Sync()

	stopCh := signals.SetupSignalHandler()

	taskRunner := loadtester.NewTaskRunner(logger, timeout, logCmdOutput)

	go taskRunner.Start(100*time.Millisecond, stopCh)

	logger.Infof("Starting load tester v%s API on port %s", VERSION, port)
	loadtester.ListenAndServe(port, time.Minute, logger, taskRunner, stopCh)
}
