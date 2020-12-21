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

package router

import flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"

const configAnnotation = "flagger.kubernetes.io/original-configuration"
const kubectlAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

type Interface interface {
	Reconcile(canary *flaggerv1.Canary) error
	SetRoutes(canary *flaggerv1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error
	GetRoutes(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, err error)
	Finalize(canary *flaggerv1.Canary) error
}
