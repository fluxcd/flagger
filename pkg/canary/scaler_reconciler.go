package canary

import (
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// ScalerReconciler represents a reconciler that can reconcile resources
// that can scale other resources.
type ScalerReconciler interface {
	ReconcilePrimaryScaler(cd *flaggerv1.Canary, init bool) error
	PauseTargetScaler(cd *flaggerv1.Canary) error
	ResumeTargetScaler(cd *flaggerv1.Canary) error
}
