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

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/canary"
)

func callWebhook(webhook string, payload interface{}, timeout string) error {
	payloadBin, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	hook, err := url.Parse(webhook)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", hook.String(), bytes.NewBuffer(payloadBin))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if timeout == "" {
		timeout = "10s"
	}

	t, err := time.ParseDuration(timeout)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(req.Context(), t)
	defer cancel()

	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %s", err.Error())
	}

	if r.StatusCode > 202 {
		return errors.New(string(b))
	}

	return nil
}

// CallWebhook does a HTTP POST to an external service and
// returns an error if the response status code is non-2xx
func CallWebhook(canary flaggerv1.Canary, phase flaggerv1.CanaryPhase, w flaggerv1.CanaryWebhook) error {
	payload := flaggerv1.CanaryWebhookPayload{
		Name:      canary.Name,
		Namespace: canary.Namespace,
		Phase:     phase,
		Checksum:  canaryChecksum(canary),
	}

	if w.Metadata != nil {
		payload.Metadata = *w.Metadata
	}

	if len(w.Timeout) < 2 {
		w.Timeout = "10s"
	}

	return callWebhook(w.URL, payload, w.Timeout)
}

func CallEventWebhook(r *flaggerv1.Canary, w flaggerv1.CanaryWebhook, message, eventtype string) error {
	t := time.Now()

	payload := flaggerv1.CanaryWebhookPayload{
		Name:      r.Name,
		Namespace: r.Namespace,
		Phase:     r.Status.Phase,
		Checksum:  canaryChecksum(*r),
		Metadata: map[string]string{
			"eventMessage": message,
			"eventType":    eventtype,
			"timestamp":    strconv.FormatInt(t.UnixNano()/1000000, 10),
		},
	}

	if w.Metadata != nil {
		for key, value := range *w.Metadata {
			if _, ok := payload.Metadata[key]; ok {
				continue
			}
			payload.Metadata[key] = value
		}
	}
	return callWebhook(w.URL, payload, "5s")
}

func canaryChecksum(c flaggerv1.Canary) string {
	canaryFields := struct {
		TrackedConfigs  *map[string]string
		LastAppliedSpec string
	}{
		c.Status.TrackedConfigs,
		c.Status.LastAppliedSpec,
	}

	return canary.ComputeHash(canaryFields)
}
