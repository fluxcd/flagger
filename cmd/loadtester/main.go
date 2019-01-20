package main

import (
	"flag"
	"github.com/knative/pkg/signals"
	"github.com/stefanprodan/flagger/pkg/loadtester"
	"github.com/stefanprodan/flagger/pkg/logging"
	"log"
	"time"
)

var (
	logLevel string
	port     string
	timeout  time.Duration
)

func init() {
	flag.StringVar(&logLevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&port, "port", "9090", "Port to listen on.")
	flag.DurationVar(&timeout, "timeout", time.Hour, "Command exec timeout.")
}

func main() {
	flag.Parse()

	logger, err := logging.NewLogger(logLevel)
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}
	defer logger.Sync()

	stopCh := signals.SetupSignalHandler()

	taskRunner := loadtester.NewTaskRunner(logger, timeout)

	go taskRunner.Start(100*time.Millisecond, stopCh)

	logger.Infof("Starting HTTP server on port %s", port)
	loadtester.ListenAndServe(port, time.Minute, logger, taskRunner, stopCh)
}
