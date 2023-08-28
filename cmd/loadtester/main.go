/*
Copyright 2020, 2022 The Flux authors

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

package main

import (
	"flag"
	"log"
	"regexp"
	"time"

	"github.com/fluxcd/flagger/pkg/loadtester"
	"github.com/fluxcd/flagger/pkg/logger"
	"github.com/fluxcd/flagger/pkg/signals"
	"go.uber.org/zap"
)

var VERSION = "0.29.0"
var (
	logLevel          string
	port              string
	timeout           time.Duration
	namespaceRegexp   string
	zapReplaceGlobals bool
	zapEncoding       string
)

func init() {
	flag.StringVar(&logLevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&port, "port", "9090", "Port to listen on.")
	flag.DurationVar(&timeout, "timeout", time.Hour, "Load test exec timeout.")
	flag.StringVar(&namespaceRegexp, "namespace-regexp", "", "Restrict access to canaries in matching namespaces.")
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

	var namespaceRegexpCompiled *regexp.Regexp
	if namespaceRegexp != "" {
		namespaceRegexpCompiled = regexp.MustCompile(namespaceRegexp)
	}
	authorizer := loadtester.NewAuthorizer(namespaceRegexpCompiled)

	loadtester.ListenAndServe(port, time.Minute, logger, taskRunner, gateStorage, authorizer, stopCh)
}
