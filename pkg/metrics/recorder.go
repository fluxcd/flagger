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

// Deployment strategies
const (
	CanaryStrategy    = "canary"
	BlueGreenStrategy = "blue_green"
	ABTestingStrategy = "ab_testing"
)

// Analysis status
const (
	AnalysisStatusCompleted = "completed"
	AnalysisStatusSkipped   = "skipped"
)

// CanaryMetricLabels holds labels for canary metrics
type CanaryMetricLabels struct {
	Name               string
	Namespace          string
	DeploymentStrategy string
	AnalysisStatus     string
}

// Values returns label values as a slice for Prometheus metrics
func (c CanaryMetricLabels) Values() []string {
	return []string{c.Name, c.Namespace, c.DeploymentStrategy, c.AnalysisStatus}
}

// Recorder records the canary analysis as Prometheus metrics
type Recorder struct {
	info      *prometheus.GaugeVec
	duration  *prometheus.HistogramVec
	total     *prometheus.GaugeVec
	status    *prometheus.GaugeVec
	weight    *prometheus.GaugeVec
	analysis  *prometheus.GaugeVec
	successes *prometheus.CounterVec
	failures  *prometheus.CounterVec
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
		Help:      "Total number of canary object",
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

	successes := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: controller,
		Name:      "canary_successes_total",
		Help:      "Total number of canary successes",
	}, []string{"name", "namespace", "deployment_strategy", "analysis_status"})

	failures := prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: controller,
		Name:      "canary_failures_total",
		Help:      "Total number of canary failures",
	}, []string{"name", "namespace", "deployment_strategy", "analysis_status"})

	if register {
		prometheus.MustRegister(info)
		prometheus.MustRegister(duration)
		prometheus.MustRegister(total)
		prometheus.MustRegister(status)
		prometheus.MustRegister(weight)
		prometheus.MustRegister(analysis)
		prometheus.MustRegister(successes)
		prometheus.MustRegister(failures)
	}

	return Recorder{
		info:      info,
		duration:  duration,
		total:     total,
		status:    status,
		weight:    weight,
		analysis:  analysis,
		successes: successes,
		failures:  failures,
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

// IncSuccesses increments the total number of canary successes
func (cr *Recorder) IncSuccesses(labels CanaryMetricLabels) {
	cr.successes.WithLabelValues(labels.Values()...).Inc()
}

// IncFailures increments the total number of canary failures
func (cr *Recorder) IncFailures(labels CanaryMetricLabels) {
	cr.failures.WithLabelValues(labels.Values()...).Inc()
}

// GetStatusMetric returns the status metric
func (cr *Recorder) GetStatusMetric() *prometheus.GaugeVec {
	return cr.status
}

// GetWeightMetric returns the weight metric
func (cr *Recorder) GetWeightMetric() *prometheus.GaugeVec {
	return cr.weight
}

// GetTotalMetric returns the total metric
func (cr *Recorder) GetTotalMetric() *prometheus.GaugeVec {
	return cr.total
}

// GetInfoMetric returns the info metric
func (cr *Recorder) GetInfoMetric() *prometheus.GaugeVec {
	return cr.info
}

// GetDurationMetric returns the duration metric
func (cr *Recorder) GetDurationMetric() *prometheus.HistogramVec {
	return cr.duration
}

// GetAnalysisMetric returns the analysis metric
func (cr *Recorder) GetAnalysisMetric() *prometheus.GaugeVec {
	return cr.analysis
}

// GetSuccessesMetric returns the successes metric
func (cr *Recorder) GetSuccessesMetric() *prometheus.CounterVec {
	return cr.successes
}

// GetFailuresMetric returns the failures metric
func (cr *Recorder) GetFailuresMetric() *prometheus.CounterVec {
	return cr.failures
}
