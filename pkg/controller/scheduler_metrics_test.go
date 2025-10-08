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

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/metrics/observers"
)

func TestController_checkMetricProviderAvailability(t *testing.T) {
	t.Run("builtin", func(t *testing.T) {
		// ok
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{Name: "request-success-rate"}}}
		canary := &flaggerv1.Canary{Spec: flaggerv1.CanarySpec{Analysis: analysis}}
		obs, err := observers.NewFactory(testMetricsServerURL)
		require.NoError(t, err)
		ctrl := Controller{observerFactory: obs, logger: zap.S(), eventRecorder: &record.FakeRecorder{}}
		require.NoError(t, ctrl.checkMetricProviderAvailability(canary))

		// error
		ctrl.observerFactory, err = observers.NewFactory("http://non-exist")
		require.NoError(t, err)
		require.Error(t, ctrl.checkMetricProviderAvailability(canary))

		// ok
		canary.Spec.MetricsServer = testMetricsServerURL
		require.NoError(t, ctrl.checkMetricProviderAvailability(canary))
	})

	t.Run("templateRef", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl

		// error (not found)
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "", TemplateRef: &flaggerv1.CrossNamespaceObjectReference{
				Name: "non-exist", Namespace: "default",
			},
		}}}
		canary := &flaggerv1.Canary{Spec: flaggerv1.CanarySpec{Analysis: analysis}}
		require.Error(t, ctrl.checkMetricProviderAvailability(canary))

		// ok
		canary.Spec.Analysis.Metrics[0].TemplateRef = &flaggerv1.CrossNamespaceObjectReference{
			Name:      "envoy",
			Namespace: "default",
		}
		require.NoError(t, ctrl.checkMetricProviderAvailability(canary))
	})

	t.Run("intraNamespaceTemplateRef", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "", TemplateRef: &flaggerv1.CrossNamespaceObjectReference{
				Name: "envoy",
			},
		}}}
		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			Spec:       flaggerv1.CanarySpec{Analysis: analysis},
		}
		require.NoError(t, ctrl.checkMetricProviderAvailability(canary))
	})
}

func TestController_runMetricChecks(t *testing.T) {
	t.Run("customVariables", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "",
			TemplateVariables: map[string]string{
				"first":  "abc",
				"second": "def",
			},
			TemplateRef: &flaggerv1.CrossNamespaceObjectReference{
				Name:      "custom-vars",
				Namespace: "default",
			},
			ThresholdRange: &flaggerv1.CanaryThresholdRange{
				Min: toFloatPtr(0),
				Max: toFloatPtr(100),
			},
		}}}
		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			Spec:       flaggerv1.CanarySpec{Analysis: analysis},
		}
		assert.Equal(t, true, ctrl.runMetricChecks(canary))
	})

	t.Run("undefined metric", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "undefined metric",
			ThresholdRange: &flaggerv1.CanaryThresholdRange{
				Min: toFloatPtr(0),
				Max: toFloatPtr(100),
			},
		}}}
		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			Spec:       flaggerv1.CanarySpec{Analysis: analysis},
		}
		assert.Equal(t, false, ctrl.runMetricChecks(canary))
	})

	t.Run("builtinMetric", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "request-success-rate",
			ThresholdRange: &flaggerv1.CanaryThresholdRange{
				Min: toFloatPtr(0),
				Max: toFloatPtr(100),
			},
		}}}
		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			Spec:       flaggerv1.CanarySpec{Analysis: analysis},
		}
		assert.Equal(t, true, ctrl.runMetricChecks(canary))
	})

	t.Run("no metric Template is defined, but a query is specified", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "undefined metric",
			ThresholdRange: &flaggerv1.CanaryThresholdRange{
				Min: toFloatPtr(0),
				Max: toFloatPtr(100),
			},
			Query: ">- sum(logback_events_total{level=\"error\", job=\"some-app\"}) <= bool 0",
		}}}
		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			Spec:       flaggerv1.CanarySpec{Analysis: analysis},
		}
		assert.Equal(t, true, ctrl.runMetricChecks(canary))
	})

	t.Run("both have metric Template and query", func(t *testing.T) {
		ctrl := newDeploymentFixture(nil).ctrl
		analysis := &flaggerv1.CanaryAnalysis{Metrics: []flaggerv1.CanaryMetric{{
			Name: "",
			TemplateVariables: map[string]string{
				"first":  "abc",
				"second": "def",
			},
			TemplateRef: &flaggerv1.CrossNamespaceObjectReference{
				Name:      "custom-vars",
				Namespace: "default",
			},
			ThresholdRange: &flaggerv1.CanaryThresholdRange{
				Min: toFloatPtr(0),
				Max: toFloatPtr(100),
			},
			Query: ">- sum(logback_events_total{level=\"error\", job=\"some-app\"}) <= bool 0",
		}}}
		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
			Spec:       flaggerv1.CanarySpec{Analysis: analysis},
		}
		assert.Equal(t, true, ctrl.runMetricChecks(canary))
	})
}

func TestController_MetricsStateTransition(t *testing.T) {
	t.Run("successful canary promotion with count metrics", func(t *testing.T) {
		mocks := newDeploymentFixture(nil)

		mocks.ctrl.advanceCanary("podinfo", "default")
		mocks.makePrimaryReady(t)
		mocks.ctrl.advanceCanary("podinfo", "default")

		actualStatus := testutil.ToFloat64(mocks.ctrl.recorder.GetStatusMetric().WithLabelValues("podinfo", "default"))
		assert.Equal(t, float64(1), actualStatus)

		actualTotal := testutil.ToFloat64(mocks.ctrl.recorder.GetTotalMetric().WithLabelValues("default"))
		assert.GreaterOrEqual(t, actualTotal, float64(0))
		dep2 := newDeploymentTestDeploymentV2()
		_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
		require.NoError(t, err)

		mocks.ctrl.advanceCanary("podinfo", "default")
		mocks.makeCanaryReady(t)
		mocks.ctrl.advanceCanary("podinfo", "default")

		actualStatus = testutil.ToFloat64(mocks.ctrl.recorder.GetStatusMetric().WithLabelValues("podinfo", "default"))
		assert.Equal(t, float64(0), actualStatus)

		actualPrimaryWeight := testutil.ToFloat64(mocks.ctrl.recorder.GetWeightMetric().WithLabelValues("podinfo-primary", "default"))
		actualCanaryWeight := testutil.ToFloat64(mocks.ctrl.recorder.GetWeightMetric().WithLabelValues("podinfo", "default"))

		t.Logf("Progression weights - Primary: %f, Canary: %f", actualPrimaryWeight, actualCanaryWeight)
		assert.GreaterOrEqual(t, actualPrimaryWeight, float64(50))
		assert.GreaterOrEqual(t, actualCanaryWeight, float64(10))
		assert.LessOrEqual(t, actualPrimaryWeight, float64(100))
		assert.LessOrEqual(t, actualCanaryWeight, float64(50))

		totalWeight := actualPrimaryWeight + actualCanaryWeight
		assert.InDelta(t, 100.0, totalWeight, 1.0)

		const maxAdvanceAttempts = 10 // sufficient attempts to reach canary completion
		for range maxAdvanceAttempts {
			mocks.ctrl.advanceCanary("podinfo", "default")
			c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
			require.NoError(t, err)
			if c.Status.Phase == flaggerv1.CanaryPhaseSucceeded {
				break
			}
		}

		successCount := testutil.ToFloat64(mocks.ctrl.recorder.GetSuccessesMetric().WithLabelValues("podinfo", "default", "canary", "completed"))
		assert.Equal(t, float64(1), successCount)
	})

	t.Run("failed canary rollback with count metrics", func(t *testing.T) {
		mocks := newDeploymentFixture(nil)

		mocks.ctrl.advanceCanary("podinfo", "default")
		mocks.makePrimaryReady(t)
		mocks.ctrl.advanceCanary("podinfo", "default")
		err := mocks.deployer.SyncStatus(mocks.canary, flaggerv1.CanaryStatus{
			Phase:        flaggerv1.CanaryPhaseProgressing,
			FailedChecks: 10,
		})
		require.NoError(t, err)

		c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		cd := c.DeepCopy()
		cd.Spec.Analysis.Metrics = append(c.Spec.Analysis.Metrics, flaggerv1.CanaryMetric{
			Name:     "fail",
			Interval: "1m",
			ThresholdRange: &flaggerv1.CanaryThresholdRange{
				Min: toFloatPtr(0),
				Max: toFloatPtr(50),
			},
			Query: "fail",
		})
		_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
		require.NoError(t, err)

		mocks.ctrl.advanceCanary("podinfo", "default")
		mocks.ctrl.advanceCanary("podinfo", "default")

		actualStatus := testutil.ToFloat64(mocks.ctrl.recorder.GetStatusMetric().WithLabelValues("podinfo", "default"))
		assert.Equal(t, float64(2), actualStatus)

		actualPrimaryWeight := testutil.ToFloat64(mocks.ctrl.recorder.GetWeightMetric().WithLabelValues("podinfo-primary", "default"))
		actualCanaryWeight := testutil.ToFloat64(mocks.ctrl.recorder.GetWeightMetric().WithLabelValues("podinfo", "default"))
		assert.Equal(t, float64(100), actualPrimaryWeight)
		assert.Equal(t, float64(0), actualCanaryWeight)

		failureCount := testutil.ToFloat64(mocks.ctrl.recorder.GetFailuresMetric().WithLabelValues("podinfo", "default", "canary", "completed"))
		assert.Equal(t, float64(1), failureCount)
	})

	t.Run("skipped analysis with count metrics", func(t *testing.T) {
		mocks := newDeploymentFixture(nil)

		mocks.ctrl.advanceCanary("podinfo", "default")
		mocks.makePrimaryReady(t)
		mocks.ctrl.advanceCanary("podinfo", "default")

		// Enable skip analysis
		cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)
		cd.Spec.SkipAnalysis = true
		_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
		require.NoError(t, err)

		// Trigger deployment update
		dep2 := newDeploymentTestDeploymentV2()
		_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
		require.NoError(t, err)

		// Skip analysis should succeed immediately
		mocks.ctrl.advanceCanary("podinfo", "default")
		mocks.makeCanaryReady(t)
		mocks.ctrl.advanceCanary("podinfo", "default")

		successCount := testutil.ToFloat64(mocks.ctrl.recorder.GetSuccessesMetric().WithLabelValues("podinfo", "default", "canary", "skipped"))
		assert.Equal(t, float64(1), successCount)
	})
}

func TestController_AnalysisMetricsRecording(t *testing.T) {
	t.Run("builtin metrics analysis recording", func(t *testing.T) {
		mocks := newDeploymentFixture(nil)

		analysis := &flaggerv1.CanaryAnalysis{
			Metrics: []flaggerv1.CanaryMetric{
				{
					Name: "request-success-rate",
					ThresholdRange: &flaggerv1.CanaryThresholdRange{
						Min: toFloatPtr(99),
						Max: toFloatPtr(100),
					},
				},
				{
					Name: "request-duration",
					ThresholdRange: &flaggerv1.CanaryThresholdRange{
						Min: toFloatPtr(0),
						Max: toFloatPtr(500),
					},
				},
			},
		}

		canary := &flaggerv1.Canary{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "podinfo",
				Namespace: "default",
			},
			Spec: flaggerv1.CanarySpec{
				TargetRef: flaggerv1.LocalObjectReference{
					Name: "podinfo",
				},
				Analysis: analysis,
			},
		}

		result := mocks.ctrl.runMetricChecks(canary)
		assert.True(t, result)

		successRateMetric := mocks.ctrl.recorder.GetAnalysisMetric().WithLabelValues("podinfo", "default", "request-success-rate")
		assert.NotNil(t, successRateMetric)

		durationMetric := mocks.ctrl.recorder.GetAnalysisMetric().WithLabelValues("podinfo", "default", "request-duration")
		assert.NotNil(t, durationMetric)
	})
}
