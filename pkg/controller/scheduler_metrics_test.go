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
