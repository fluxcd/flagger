package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
}
