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

// NopTracker no-operation tracker
type NopTracker struct{}

func (nt *NopTracker) GetTargetConfigs(*flaggerv1.Canary) (map[string]ConfigRef, error) {
	res := make(map[string]ConfigRef)
	return res, nil
}

func (nt *NopTracker) GetConfigRefs(*flaggerv1.Canary) (*map[string]string, error) {
	return nil, nil
}

func (nt *NopTracker) HasConfigChanged(*flaggerv1.Canary) (bool, error) {
	return false, nil
}

func (nt *NopTracker) CreatePrimaryConfigs(*flaggerv1.Canary, map[string]ConfigRef, []string) error {
	return nil
}

func (nt *NopTracker) ApplyPrimaryConfigs(spec corev1.PodSpec, _ map[string]ConfigRef) corev1.PodSpec {
	return spec
}
