package canary

import "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"

type Controller interface {
	IsPrimaryReady(canary *v1alpha3.Canary) (bool, error)
	IsCanaryReady(canary *v1alpha3.Canary) (bool, error)
	SyncStatus(canary *v1alpha3.Canary, status v1alpha3.CanaryStatus) error
	SetStatusFailedChecks(canary *v1alpha3.Canary, val int) error
	SetStatusWeight(canary *v1alpha3.Canary, val int) error
	SetStatusIterations(canary *v1alpha3.Canary, val int) error
	SetStatusPhase(canary *v1alpha3.Canary, phase v1alpha3.CanaryPhase) error
	Initialize(canary *v1alpha3.Canary, skipLivenessChecks bool) (label string, ports map[string]int32, err error)
	Promote(canary *v1alpha3.Canary) error
	HasTargetChanged(canary *v1alpha3.Canary) (bool, error)
	HaveDependenciesChanged(canary *v1alpha3.Canary) (bool, error)
	Scale(canary *v1alpha3.Canary, replicas int32) error
	ScaleFromZero(canary *v1alpha3.Canary) error
}
