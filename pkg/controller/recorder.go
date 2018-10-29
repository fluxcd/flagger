package controller

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
)

// CanaryRecorder records the canary analysis as Prometheus metrics
type CanaryRecorder struct {
	status *prometheus.GaugeVec
	weight *prometheus.GaugeVec
}

// NewCanaryRecorder registers the Prometheus metrics
func NewCanaryRecorder() CanaryRecorder {
	// 0 - running, 1 - successful, 2 - failed
	status := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controllerAgentName,
		Name:      "canary_status",
		Help:      "Last canary analysis result",
	}, []string{"name", "namespace"})
	prometheus.MustRegister(status)

	weight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: controllerAgentName,
		Name:      "canary_weight",
		Help:      "The virtual service destination weight current value",
	}, []string{"workload", "namespace"})
	prometheus.MustRegister(weight)

	return CanaryRecorder{
		status: status,
		weight: weight,
	}
}

// RecordStatus sets the last known canary analysis status
func (cr *CanaryRecorder) RecordStatus(cd *flaggerv1.Canary) {
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

// RecordWeight sets the weight values for primary and canary destinations
func (cr *CanaryRecorder) RecordWeight(cd *flaggerv1.Canary, primary int, canary int) {
	cr.weight.WithLabelValues(fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name), cd.Namespace).Set(float64(primary))
	cr.weight.WithLabelValues(cd.Spec.TargetRef.Name, cd.Namespace).Set(float64(canary))
}
