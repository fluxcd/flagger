// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package glog exposes an API subset of the [glog](https://github.com/golang/glog) package.
// All logging state delivered to this package is shunted to the global [zap logger](https://github.com/uber-go/zap).
//
// Istio is built on top of zap logger. We depend on some downstream components that use glog for logging.
// This package makes it so we can intercept the calls to glog and redirect them to zap and thus produce
// a consistent log for our processes.
package klog

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// loggingT collects all the global state of the logging setup.
type loggingT struct {
	// mu protects the remaining elements of this structure
	mu        sync.RWMutex
	verbosity Level // V logging level, the value of the -v flag/

	// zapGlobal is a reference to the most recently observed zap global
	// logger.
	zapGlobal *zap.Logger
	// logger is a cached logger with the correct caller skip level.
	logger *zap.SugaredLogger
}

var logging loggingT

// Level is exported because it appears in the arguments to V and is
// the type of the v flag, which can be set programmatically.
// It's a distinct type because we want to discriminate it from logType.
// Variables of type level are only changed under logging.mu.
// The -v flag is read only with atomic ops, so the state of the logging
// module is consistent.

// Level is treated as a sync/atomic int32.

// Level specifies a level of verbosity for V logs. *Level implements
// flag.Value; the -v flag is of type Level and should be modified
// only through the flag.Value interface.
type Level int32

// get returns the value of the Level.
func (l *Level) get() Level {
	return Level(atomic.LoadInt32((*int32)(l)))
}

// set sets the value of the Level.
func (l *Level) set(val Level) {
	atomic.StoreInt32((*int32)(l), int32(val))
}

// String is part of the flag.Value interface.
func (l *Level) String() string {
	return strconv.FormatInt(int64(*l), 10)
}

// Get is part of the flag.Value interface.
func (l *Level) Get() interface{} {
	return *l
}

// Set is part of the flag.Value interface.
func (l *Level) Set(value string) error {
	v, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	logging.mu.Lock()
	defer logging.mu.Unlock()
	logging.verbosity.set(Level(v))
	return nil
}

// Verbose is a shim
type Verbose bool

// Flush is a shim
func Flush() {
	zap.L().Sync()
}

// skipLogger constructs a suggared logger with an additional caller skip to
// ensure we log the correct line number.
func skipLogger() *zap.SugaredLogger {
	logging.mu.RLock()
	cachedGlobal := logging.zapGlobal
	cachedLogger := logging.logger
	logging.mu.RUnlock()

	global := zap.L()
	if cachedGlobal != global {
		logging.mu.Lock()
		logging.zapGlobal = global
		logging.logger = global.WithOptions(zap.AddCallerSkip(1)).Sugar()
		cachedLogger = logging.logger
		logging.mu.Unlock()
	}

	return cachedLogger
}

// max calculates the maximum of the two given Levels.
func max(a, b Level) Level {
	if a > b {
		return a
	}
	return b
}

// V reports whether verbosity at the call site is at least the requested level.
// The returned value is a boolean of type Verbose, which implements Info, Infoln
// and Infof. These methods will write to the Info log if called.
//
// Verbosity is controlled both by the -v flag and the zap log level.
func V(level Level) Verbose {
	lvl := logging.verbosity.get()
	core := zap.L().Core()
	if core.Enabled(zap.DebugLevel) {
		return Verbose(level <= max(Level(4), lvl))
	}
	if core.Enabled(zap.InfoLevel) {
		return Verbose(level <= max(Level(2), lvl))
	}
	return Verbose(level <= lvl)
}

// Info is a shim
func (v Verbose) Info(args ...interface{}) {
	if v {
		skipLogger().Info(args...)
	}
}

// Infoln is a shim
func (v Verbose) Infoln(args ...interface{}) {
	if v {
		skipLogger().Info(fmt.Sprint(args), "\n")
	}
}

// Infof is a shim
func (v Verbose) Infof(format string, args ...interface{}) {
	if v {
		skipLogger().Infof(format, args...)
	}
}

// Info is a shim
func Info(args ...interface{}) {
	skipLogger().Info(args...)
}

// InfoDepth is a shim
func InfoDepth(depth int, args ...interface{}) {
	skipLogger().Info(args...)
}

// Infoln is a shim
func Infoln(args ...interface{}) {
	s := fmt.Sprint(args)
	skipLogger().Info(s, "\n")
}

// Infof is a shim
func Infof(format string, args ...interface{}) {
	skipLogger().Infof(format, args...)
}

// Warning is a shim
func Warning(args ...interface{}) {
	skipLogger().Warn(args...)
}

// WarningDepth is a shim
func WarningDepth(depth int, args ...interface{}) {
	skipLogger().Warn(args...)
}

// Warningln is a shim
func Warningln(args ...interface{}) {
	s := fmt.Sprint(args)
	skipLogger().Warn(s, "\n")
}

// Warningf is a shim
func Warningf(format string, args ...interface{}) {
	skipLogger().Warnf(format, args...)
}

// Error is a shim
func Error(args ...interface{}) {
	skipLogger().Error(args...)
}

// ErrorDepth is a shim
func ErrorDepth(depth int, args ...interface{}) {
	skipLogger().Error(args...)
}

// Errorln is a shim
func Errorln(args ...interface{}) {
	s := fmt.Sprint(args)
	skipLogger().Error(s, "\n")
}

// Errorf is a shim
func Errorf(format string, args ...interface{}) {
	skipLogger().Errorf(format, args...)
}

// Fatal is a shim
func Fatal(args ...interface{}) {
	skipLogger().Error(args...)
	os.Exit(255)
}

// FatalDepth is a shim
func FatalDepth(depth int, args ...interface{}) {
	skipLogger().Error(args...)
	os.Exit(255)
}

// Fatalln is a shim
func Fatalln(args ...interface{}) {
	s := fmt.Sprint(args)
	skipLogger().Error(s, "\n")
	os.Exit(255)
}

// Fatalf is a shim
func Fatalf(format string, args ...interface{}) {
	skipLogger().Errorf(format, args...)
	os.Exit(255)
}

// Exit is a shim
func Exit(args ...interface{}) {
	skipLogger().Error(args...)
	os.Exit(1)
}

// ExitDepth is a shim
func ExitDepth(depth int, args ...interface{}) {
	skipLogger().Error(args...)
	os.Exit(1)
}

// Exitln is a shim
func Exitln(args ...interface{}) {
	s := fmt.Sprint(args)
	skipLogger().Error(s, "\n")
	os.Exit(1)
}

// Exitf is a shim
func Exitf(format string, args ...interface{}) {
	skipLogger().Errorf(format, args...)
	os.Exit(1)
}
