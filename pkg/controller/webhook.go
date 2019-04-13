package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

// CallWebhook does a HTTP POST to an external service and
// returns an error if the response status code is non-2xx
func CallWebhook(name string, namespace string, phase flaggerv1.CanaryPhase, w flaggerv1.CanaryWebhook) error {
	payload := flaggerv1.CanaryWebhookPayload{
		Name:      name,
		Namespace: namespace,
		Phase:     phase,
	}

	if w.Metadata != nil {
		payload.Metadata = *w.Metadata
	}

	payloadBin, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	hook, err := url.Parse(w.URL)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", hook.String(), bytes.NewBuffer(payloadBin))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if len(w.Timeout) < 2 {
		w.Timeout = "10s"
	}

	timeout, err := time.ParseDuration(w.Timeout)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	r, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %s", err.Error())
	}

	if r.StatusCode > 202 {
		return errors.New(string(b))
	}

	return nil
}
