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
	corev1 "k8s.io/api/core/v1"
)

type Tracker interface {
	GetTargetConfigs(cd *flaggerv1.Canary) (map[string]ConfigRef, error)
	GetConfigRefs(cd *flaggerv1.Canary) (*map[string]string, error)
	HasConfigChanged(cd *flaggerv1.Canary) (bool, error)
	CreatePrimaryConfigs(cd *flaggerv1.Canary, refs map[string]ConfigRef, includeLabelPrefix []string) error
	ApplyPrimaryConfigs(spec corev1.PodSpec, refs map[string]ConfigRef) corev1.PodSpec
}
