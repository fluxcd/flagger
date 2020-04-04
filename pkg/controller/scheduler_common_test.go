package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

func assertPhase(flaggerClient clientset.Interface, canary string, phase flaggerv1.CanaryPhase) error {
	c, err := flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), canary, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if c.Status.Phase != phase {
		return fmt.Errorf("Got canary state %s wanted %s", c.Status.Phase, phase)
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
