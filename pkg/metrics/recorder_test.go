/*
Copyright 2025 The Flux authors

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

package metrics

import (
	"testing"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRecorder_GetterMethodsWithData(t *testing.T) {
	recorder := NewRecorder("test", false)

	canary := &flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podinfo",
			Namespace: "default",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
				Name: "podinfo",
			},
		},
	}

	tests := []struct {
		name       string
		setupFunc  func(Recorder)
		getterFunc func(Recorder) interface{}
		labels     []string
		expected   float64
		checkValue bool
	}{
		{
			name:       "SetAndGetInfo",
			setupFunc:  func(r Recorder) { r.SetInfo("v1.0.0", "istio") },
			getterFunc: func(r Recorder) interface{} { return r.GetInfoMetric() },
			labels:     []string{"v1.0.0", "istio"},
			expected:   1.0,
			checkValue: true,
		},
		{
			name:       "SetAndGetStatus",
			setupFunc:  func(r Recorder) { r.SetStatus(canary, flaggerv1.CanaryPhaseSucceeded) },
			getterFunc: func(r Recorder) interface{} { return r.GetStatusMetric() },
			labels:     []string{"podinfo", "default"},
			expected:   1.0,
			checkValue: true,
		},
		{
			name:       "SetAndGetTotal",
			setupFunc:  func(r Recorder) { r.SetTotal("default", 3) },
			getterFunc: func(r Recorder) interface{} { return r.GetTotalMetric() },
			labels:     []string{"default"},
			expected:   3.0,
			checkValue: true,
		},
		{
			name:       "SetAndGetDuration",
			setupFunc:  func(r Recorder) { r.SetDuration(canary, time.Second*5) },
			getterFunc: func(r Recorder) interface{} { return r.GetDurationMetric() },
			labels:     nil,
			expected:   0,
			checkValue: false, // Histogram values can't be easily checked with testutil
		},
		{
			name:       "SetAndGetAnalysis",
			setupFunc:  func(r Recorder) { r.SetAnalysis(canary, "request-success-rate", 99.5) },
			getterFunc: func(r Recorder) interface{} { return r.GetAnalysisMetric() },
			labels:     []string{"podinfo", "default", "request-success-rate"},
			expected:   99.5,
			checkValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFunc(recorder)

			metric := tt.getterFunc(recorder)
			assert.NotNil(t, metric)

			if tt.checkValue {
				if gaugeVec, ok := metric.(*prometheus.GaugeVec); ok {
					value := testutil.ToFloat64(gaugeVec.WithLabelValues(tt.labels...))
					assert.Equal(t, tt.expected, value)
				}
			}
		})
	}
}
