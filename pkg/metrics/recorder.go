package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// Recorder records the canary analysis as Prometheus metrics
type Recorder struct {
	info     *prometheus.GaugeVec
	duration *prometheus.HistogramVec
	total    *prometheus.GaugeVec
	status   *prometheus.GaugeVec
	weight   *prometheus.GaugeVec
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

	if register {
		prometheus.MustRegister(info)
		prometheus.MustRegister(duration)
		prometheus.MustRegister(total)
		prometheus.MustRegister(status)
		prometheus.MustRegister(weight)
	}

	return Recorder{
		info:     info,
		duration: duration,
		total:    total,
		status:   status,
		weight:   weight,
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
