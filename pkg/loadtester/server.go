package loadtester

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	"go.uber.org/zap"
)

// ListenAndServe starts a web server and waits for SIGTERM
func ListenAndServe(port string, timeout time.Duration, logger *zap.SugaredLogger, taskRunner *TaskRunner, stopCh <-chan struct{}) {
	mux := http.DefaultServeMux
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
	})
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 1 * time.Minute,
		IdleTimeout:  15 * time.Second,
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
