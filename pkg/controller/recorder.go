package controller

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
)

// CanaryRecorder records the canary analysis as Prometheus metrics
type CanaryRecorder struct {
	duration *prometheus.HistogramVec
	total    *prometheus.GaugeVec
	status   *prometheus.GaugeVec
	weight   *prometheus.GaugeVec
}

// NewCanaryRecorder creates a new recorder and registers the Prometheus metrics
func NewCanaryRecorder(register bool) CanaryRecorder {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: controllerAgentName,
		Name:      "canary_duration_seconds",
		Help:      "Seconds spent performing canary analysis.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"name", "namespace"})

	total := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controllerAgentName,
		Name:      "canary_total",
		Help:      "Total number of canary object",
	}, []string{"namespace"})

	// 0 - running, 1 - successful, 2 - failed
	status := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controllerAgentName,
		Name:      "canary_status",
		Help:      "Last canary analysis result",
	}, []string{"name", "namespace"})

	weight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controllerAgentName,
		Name:      "canary_weight",
		Help:      "The virtual service destination weight current value",
	}, []string{"workload", "namespace"})

	if register {
		prometheus.MustRegister(duration)
		prometheus.MustRegister(total)
		prometheus.MustRegister(status)
		prometheus.MustRegister(weight)
	}

	return CanaryRecorder{
		duration: duration,
		total:    total,
		status:   status,
		weight:   weight,
	}
}

// SetDuration sets the time spent in seconds performing canary analysis
func (cr *CanaryRecorder) SetDuration(cd *flaggerv1.Canary, duration time.Duration) {
	cr.duration.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Observe(duration.Seconds())
}

// SetTotal sets the total number of canaries per namespace
func (cr *CanaryRecorder) SetTotal(namespace string, total int) {
	cr.total.WithLabelValues(namespace).Set(float64(total))
}

// SetStatus sets the last known canary analysis status
func (cr *CanaryRecorder) SetStatus(cd *flaggerv1.Canary) {
	status := 1
	switch cd.Status.State {
	case "running":
		status = 0
	case "failed":
		status = 2
	default:
		status = 1
	}
	cr.status.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Set(float64(status))
}

// SetWeight sets the weight values for primary and canary destinations
func (cr *CanaryRecorder) SetWeight(cd *flaggerv1.Canary, primary int, canary int) {
	cr.weight.WithLabelValues(fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name), cd.Namespace).Set(float64(primary))
	cr.weight.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Set(float64(canary))
}
