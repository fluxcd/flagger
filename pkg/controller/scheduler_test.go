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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testMetricsServerURL string

func TestMain(m *testing.M) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query()["query"][0] == "vector(1)" {
			// for IsOnline invoked during canary initialization
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"1"]}]}}`))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`))
	}))

	testMetricsServerURL = ts.URL
	defer ts.Close()
	os.Exit(m.Run())
}

func assertPhase(flaggerClient clientset.Interface, canary string, phase flaggerv1.CanaryPhase) error {
	c, err := flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), canary, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if c.Status.Phase != phase {
		return fmt.Errorf("got canary state %s wanted %s", c.Status.Phase, phase)
	}

	return nil
}

func alwaysReady() bool {
	return true
}

func toFloatPtr(val int) *float64 {
	v := float64(val)
	return &v
}
