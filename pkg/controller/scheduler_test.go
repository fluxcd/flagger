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
