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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const (
	totalWeight = 100
)

func TestScheduler_ServicePromotion(t *testing.T) {
	testServicePromotion(t, newTestServiceCanary(), []int{totalWeight, 80, 60, 40})
}

func TestScheduler_ServicePromotionMaxWeight(t *testing.T) {
	testServicePromotion(t, newTestServiceCanaryMaxWeight(), []int{totalWeight, 50, 0})
}

func TestScheduler_ServicePromotionWithWeightsHappyCase(t *testing.T) {
	testServicePromotion(t, newTestServiceCanaryWithWeightsHappyCase(), []int{totalWeight, 99, 98, 90, 20})
}

func TestScheduler_ServicePromotionWithWeightsOverflow(t *testing.T) {
	testServicePromotion(t, newTestServiceCanaryWithWeightsOverflow(), []int{totalWeight, 99, 98, 90, 0})
}

func testServicePromotion(t *testing.T, canary *flaggerv1.Canary, expectedPrimaryWeigths []int) {
	mocks := newDeploymentFixture(canary)

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

	for _, expectedPrimaryWeigth := range expectedPrimaryWeigths {
		mocks.ctrl.advanceCanary("podinfo", "default")
		expectedCanaryWeight := totalWeight - expectedPrimaryWeigth
		primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
		require.NoError(t, err)
		assert.Equal(t, expectedPrimaryWeigth, primaryWeight)
		assert.Equal(t, expectedCanaryWeight, canaryWeight)
		assert.False(t, mirrored)
	}

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

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, totalWeight, primaryWeight)
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
				StepWeight: 20,
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

func newTestServiceCanaryMaxWeight() *flaggerv1.Canary {
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
				StepWeight: 50,
				MaxWeight:  totalWeight,
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

func newTestServiceCanaryWithWeightsHappyCase() *flaggerv1.Canary {
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
				Threshold:   10,
				StepWeights: []int{1, 2, 10, 80},
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

func newTestServiceCanaryWithWeightsOverflow() *flaggerv1.Canary {
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
				Threshold:   10,
				StepWeights: []int{1, 2, 10, totalWeight + 100},
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
