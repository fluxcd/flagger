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

package metrics

import (
	"fmt"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
)

// Recorder records the canary analysis as Prometheus metrics
type Recorder struct {
	info     *prometheus.GaugeVec
	duration *prometheus.HistogramVec
	total    *prometheus.GaugeVec
	status   *prometheus.GaugeVec
	weight   *prometheus.GaugeVec
	analysis *prometheus.GaugeVec
	failure_total *prometheus.CounterVec
	success_total *prometheus.CounterVec
}

// NewRecorder creates a new recorder and registers the Prometheus metrics
func NewRecorder(controller string, register bool) Recorder {
	info := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controller,
		Name:      "info",
		Help:      "Flagger version and mesh provider information",
	}, []string{"version", "mesh_provider"})

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: controller,
		Name:      "canary_duration_seconds",
		Help:      "Seconds spent performing canary analysis.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"name", "namespace"})

	total := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controller,
		Name:      "canary_total",
		Help:      "Total number of active canary object",
	}, []string{"namespace"})

	// 0 - running, 1 - successful, 2 - failed
	status := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controller,
		Name:      "canary_status",
		Help:      "Last canary analysis result",
	}, []string{"name", "namespace"})

	weight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controller,
		Name:      "canary_weight",
		Help:      "The virtual service destination weight current value",
	}, []string{"workload", "namespace"})

	analysis := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controller,
		Name:      "canary_metric_analysis",
		Help:      "Last canary analysis result per metric",
	}, []string{"name", "namespace", "metric"})

	failure_total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: controller,
		Name:      "count_canary_failure",
		Help:      "Total number of canary failures",
	}, []string{"name", "namespace"})

	success_total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: controller,
		Name:      "count_canary_success",
		Help:      "Total number of canary successes",
	}, []string{"name", "namespace"})

	if register {
		prometheus.MustRegister(info)
		prometheus.MustRegister(duration)
		prometheus.MustRegister(total)
		prometheus.MustRegister(status)
		prometheus.MustRegister(weight)
		prometheus.MustRegister(analysis)
		prometheus.MustRegister(failure_total)
		prometheus.MustRegister(success_total)
	}

	return Recorder{
		info:     info,
		duration: duration,
		total:    total,
		status:   status,
		weight:   weight,
		analysis: analysis,
		failure_total: failure_total,
		success_total: success_total,
	}
}

// SetInfo sets the version and mesh provider labels
func (cr *Recorder) SetInfo(version string, meshProvider string) {
	cr.info.WithLabelValues(version, meshProvider).Set(1)
}

// SetDuration sets the time spent in seconds performing canary analysis
func (cr *Recorder) SetDuration(cd *flaggerv1.Canary, duration time.Duration) {
	cr.duration.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Observe(duration.Seconds())
}

// SetTotal sets the total number of canaries per namespace
func (cr *Recorder) SetTotal(namespace string, total int) {
	cr.total.WithLabelValues(namespace).Set(float64(total))
 }

func (cr *Recorder) SetAnalysis(cd *flaggerv1.Canary, metricTemplateName string, val float64) {
	cr.analysis.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace, metricTemplateName).Set(val)
}

// IncFailure increments the the canary failures
func (cr *Recorder) IncFailure(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) {
	cr.failure_total.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Inc()
}

func (cr *Recorder) IncSuccess(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) {
	cr.success_total.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Inc()
}

// SetStatus sets the last known canary analysis status
func (cr *Recorder) SetStatus(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) {
	var status int
	switch phase {
	case flaggerv1.CanaryPhaseProgressing:
		status = 0
	case flaggerv1.CanaryPhaseFailed:
		status = 2
	default:
		status = 1
	}
	cr.status.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Set(float64(status))
}

// SetWeight sets the weight values for primary and canary destinations
func (cr *Recorder) SetWeight(cd *flaggerv1.Canary, primary int, canary int) {
	cr.weight.WithLabelValues(fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name), cd.Namespace).Set(float64(primary))
	cr.weight.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Set(float64(canary))
}
