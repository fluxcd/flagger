package canary

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

type Controller interface {
	IsPrimaryReady(canary *flaggerv1.Canary) (bool, error)
	IsCanaryReady(canary *flaggerv1.Canary) (bool, error)
	GetMetadata(canary *flaggerv1.Canary) (string, map[string]int32, error)
	SyncStatus(canary *flaggerv1.Canary, status flaggerv1.CanaryStatus) error
	SetStatusFailedChecks(canary *flaggerv1.Canary, val int) error
	SetStatusWeight(canary *flaggerv1.Canary, val int) error
	SetStatusIterations(canary *flaggerv1.Canary, val int) error
	SetStatusPhase(canary *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error
	Initialize(canary *flaggerv1.Canary, skipLivenessChecks bool) error
	Promote(canary *flaggerv1.Canary) error
	HasTargetChanged(canary *flaggerv1.Canary) (bool, error)
	HaveDependenciesChanged(canary *flaggerv1.Canary) (bool, error)
	Scale(canary *flaggerv1.Canary, replicas int32) error
	ScaleFromZero(canary *flaggerv1.Canary) error
}
