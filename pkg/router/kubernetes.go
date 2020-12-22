package router

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// KubernetesRouter manages Kubernetes services
type KubernetesRouter interface {
	// Initialize creates or updates the primary and canary services
	Initialize(canary *flaggerv1.Canary) error
	// Reconcile creates or updates the main service
	Reconcile(canary *flaggerv1.Canary) error
	// Revert router
	Finalize(canary *flaggerv1.Canary) error
}
