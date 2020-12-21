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

package canary

import (
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

type Controller interface {
	IsPrimaryReady(canary *flaggerv1.Canary) error
	IsCanaryReady(canary *flaggerv1.Canary) (bool, error)
	GetMetadata(canary *flaggerv1.Canary) (string, string, map[string]int32, error)
	SyncStatus(canary *flaggerv1.Canary, status flaggerv1.CanaryStatus) error
	SetStatusFailedChecks(canary *flaggerv1.Canary, val int) error
	SetStatusWeight(canary *flaggerv1.Canary, val int) error
	SetStatusIterations(canary *flaggerv1.Canary, val int) error
	SetStatusPhase(canary *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error
	Initialize(canary *flaggerv1.Canary) error
	Promote(canary *flaggerv1.Canary) error
	HasTargetChanged(canary *flaggerv1.Canary) (bool, error)
	HaveDependenciesChanged(canary *flaggerv1.Canary) (bool, error)
	ScaleToZero(canary *flaggerv1.Canary) error
	ScaleFromZero(canary *flaggerv1.Canary) error
	Finalize(canary *flaggerv1.Canary) error
}
