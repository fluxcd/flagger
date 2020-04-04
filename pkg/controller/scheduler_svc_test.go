package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestScheduler_ServicePromotion(t *testing.T) {
	mocks := newDeploymentFixture(newTestServiceCanary())

	// init
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check initialized status
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseInitialized, c.Status.Phase)

	// update
	svc2 := newDeploymentTestServiceV2()
	_, err = mocks.kubeClient.CoreV1().Services("default").Update(context.TODO(), svc2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect service spec changes
	mocks.ctrl.advanceCanary("podinfo", "default")

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	primaryWeight = 60
	canaryWeight = 40
	err = mocks.router.SetRoutes(mocks.canary, primaryWeight, canaryWeight, mirrored)
	require.NoError(t, err)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check progressing status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseProgressing, c.Status.Phase)

	// promote
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check promoting status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhasePromoting, c.Status.Phase)

	// finalise
	mocks.ctrl.advanceCanary("podinfo", "default")

	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, primaryWeight)
	assert.Equal(t, 0, canaryWeight)
	assert.False(t, mirrored)

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	primaryLabelValue := primarySvc.Spec.Selector["app"]
	canaryLabelValue := svc2.Spec.Selector["app"]
	assert.Equal(t, canaryLabelValue, primaryLabelValue)

	// check finalising status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseFinalising, c.Status.Phase)

	// scale canary to zero
	mocks.ctrl.advanceCanary("podinfo", "default")

	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseSucceeded, c.Status.Phase)
}

func newTestServiceCanary() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "core/v1",
				Kind:       "Service",
			},
			Service: flaggerv1.CanaryService{
				Port: 9898,
			},
			Analysis: &flaggerv1.CanaryAnalysis{
				Threshold:  10,
				StepWeight: 10,
				MaxWeight:  50,
				Metrics: []flaggerv1.CanaryMetric{
					{
						Name:      "request-success-rate",
						Threshold: 99,
						Interval:  "1m",
					},
					{
						Name:      "request-duration",
						Threshold: 500000,
						Interval:  "1m",
					},
				},
			},
		},
	}
	return cd
}
