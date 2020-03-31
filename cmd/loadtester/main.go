package main

import (
	"flag"
	"log"
	"time"

	"github.com/weaveworks/flagger/pkg/loadtester"
	"github.com/weaveworks/flagger/pkg/logger"
	"github.com/weaveworks/flagger/pkg/signals"
	"go.uber.org/zap"
)

var VERSION = "0.16.0"
var (
	logLevel          string
	port              string
	timeout           time.Duration
	zapReplaceGlobals bool
	zapEncoding       string
)

func init() {
	flag.StringVar(&logLevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&port, "port", "9090", "Port to listen on.")
	flag.DurationVar(&timeout, "timeout", time.Hour, "Load test exec timeout.")
	flag.BoolVar(&zapReplaceGlobals, "zap-replace-globals", false, "Whether to change the logging level of the global zap logger.")
	flag.StringVar(&zapEncoding, "zap-encoding", "json", "Zap logger encoding.")
}

func main() {
	flag.Parse()

	logger, err := logger.NewLoggerWithEncoding(logLevel, zapEncoding)
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}
	if zapReplaceGlobals {
		zap.ReplaceGlobals(logger.Desugar())
	}

	defer logger.Sync()

	stopCh := signals.SetupSignalHandler()

	taskRunner := loadtester.NewTaskRunner(logger, timeout)

	go taskRunner.Start(100*time.Millisecond, stopCh)

	logger.Infof("Starting load tester v%s API on port %s", VERSION, port)

	gateStorage := loadtester.NewGateStorage("in-memory")
	loadtester.ListenAndServe(port, time.Minute, logger, taskRunner, gateStorage, stopCh)
}
