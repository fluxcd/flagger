package loadtester

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"go.uber.org/zap"
)

// ListenAndServe starts a web server and waits for SIGTERM
func ListenAndServe(port string, timeout time.Duration, logger *zap.SugaredLogger, taskRunner *TaskRunner, gate *GateStorage, stopCh <-chan struct{}) {
	mux := http.DefaultServeMux
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", HandleHealthz)
	mux.HandleFunc("/gate/approve", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/gate/halt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Forbidden"))
	})
	mux.HandleFunc("/gate/check", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		canary := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, canary)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		canaryName := fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)
		approved := gate.isOpen(canaryName)
		if approved {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Approved"))
		} else {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Forbidden"))
		}

		logger.Infof("%s gate check: approved %v", canaryName, approved)
	})

	mux.HandleFunc("/gate/open", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		canary := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, canary)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		canaryName := fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)
		gate.open(canaryName)

		w.WriteHeader(http.StatusAccepted)

		logger.Infof("%s gate opened", canaryName)
	})

	mux.HandleFunc("/gate/close", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		canary := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, canary)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		canaryName := fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)
		gate.close(canaryName)

		w.WriteHeader(http.StatusAccepted)

		logger.Infof("%s gate closed", canaryName)
	})

	mux.HandleFunc("/rollback/check", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		canary := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, canary)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		canaryName := fmt.Sprintf("rollback.%s.%s", canary.Name, canary.Namespace)
		approved := gate.isOpen(canaryName)
		if approved {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Approved"))
		} else {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Forbidden"))
		}

		logger.Infof("%s rollback check: approved %v", canaryName, approved)
	})
	mux.HandleFunc("/rollback/open", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		canary := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, canary)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		canaryName := fmt.Sprintf("rollback.%s.%s", canary.Name, canary.Namespace)
		gate.open(canaryName)

		w.WriteHeader(http.StatusAccepted)

		logger.Infof("%s rollback opened", canaryName)
	})
	mux.HandleFunc("/rollback/close", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		canary := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, canary)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		canaryName := fmt.Sprintf("rollback.%s.%s", canary.Name, canary.Namespace)
		gate.close(canaryName)

		w.WriteHeader(http.StatusAccepted)

		logger.Infof("%s rollback closed", canaryName)
	})

	mux.HandleFunc("/", HandleNewTask(logger, taskRunner))
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// run server in background
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Fatalf("HTTP server crashed %v", err)
		}
	}()

	// wait for SIGTERM or SIGINT
	<-stopCh
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("HTTP server graceful shutdown failed %v", err)
	} else {
		logger.Info("HTTP server stopped")
	}
}

// HandleHealthz handles heath check requests
func HandleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// HandleNewTask handles task creation requests
func HandleNewTask(logger *zap.SugaredLogger, taskRunner TaskRunnerInterface) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Error("reading the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		payload := &flaggerv1.CanaryWebhookPayload{}
		err = json.Unmarshal(body, payload)
		if err != nil {
			logger.Error("decoding the request body failed", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if len(payload.Metadata) > 0 {
			metadata := payload.Metadata
			var typ, ok = metadata["type"]
			if !ok {
				typ = TaskTypeShell
			}

			rtnCmdOutput := false
			if rtn, ok := metadata["returnCmdOutput"]; ok {
				rtnCmdOutput, err = strconv.ParseBool(rtn)
			}

			// run bash command (blocking task)
			if typ == TaskTypeBash {
				logger.With("canary", payload.Name).Infof("bash command %s", payload.Metadata["cmd"])

				bashTask := BashTask{
					command:      payload.Metadata["cmd"],
					logCmdOutput: true,
					TaskBase: TaskBase{
						canary: fmt.Sprintf("%s.%s", payload.Name, payload.Namespace),
						logger: logger,
					},
				}

				ctx, cancel := context.WithTimeout(context.Background(), taskRunner.Timeout())
				defer cancel()

				result, err := bashTask.Run(ctx)
				if !result.ok {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}

				w.WriteHeader(http.StatusOK)
				if rtnCmdOutput {
					w.Write(result.out)
				}
				return
			}

			// run helm command (blocking task)
			if typ == TaskTypeHelm {
				helm := HelmTask{
					command:      payload.Metadata["cmd"],
					logCmdOutput: true,
					TaskBase: TaskBase{
						canary: fmt.Sprintf("%s.%s", payload.Name, payload.Namespace),
						logger: logger,
					},
				}

				ctx, cancel := context.WithTimeout(context.Background(), taskRunner.Timeout())
				defer cancel()

				result, err := helm.Run(ctx)
				if !result.ok {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}

				w.WriteHeader(http.StatusOK)
				if rtnCmdOutput {
					w.Write(result.out)
				}
				return
			}

			// run helmv3 command (blocking task)
			if typ == TaskTypeHelmv3 {
				helm := HelmTaskv3{
					command:      payload.Metadata["cmd"],
					logCmdOutput: true,
					TaskBase: TaskBase{
						canary: fmt.Sprintf("%s.%s", payload.Name, payload.Namespace),
						logger: logger,
					},
				}

				ctx, cancel := context.WithTimeout(context.Background(), taskRunner.Timeout())
				defer cancel()

				result, err := helm.Run(ctx)
				if !result.ok {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}

				w.WriteHeader(http.StatusOK)
				if rtnCmdOutput {
					w.Write(result.out)
				}
				return
			}

			// run concord job (blocking task)
			if typ == TaskTypeConcord {
				concord, err := NewConcordTask(payload.Metadata, fmt.Sprintf("%s.%s", payload.Name, payload.Namespace), logger)

				if err != nil {
					logger.With("canary", payload.Name).Errorf("concord task init error: %s", err)
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}

				ctx, cancel := context.WithTimeout(context.Background(), taskRunner.Timeout())
				defer cancel()

				result, err := concord.Run(ctx)
				if !result.ok {
					if err != nil {
						logger.With("canary", payload.Name).Errorf("concord task error: %s", err)
					}
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}

				w.WriteHeader(http.StatusOK)
				if rtnCmdOutput {
					w.Write(result.out)
				}
				return
			}

			taskFactory, ok := GetTaskFactory(typ)
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(fmt.Sprintf("unknown task type %s", typ)))
				return
			}
			canary := fmt.Sprintf("%s.%s", payload.Name, payload.Namespace)
			task, err := taskFactory(metadata, canary, logger)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				return
			}
			taskRunner.Add(task)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("metadata not found in payload"))
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}
