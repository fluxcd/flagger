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

import (
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// NopRouter no-operation router
type NopRouter struct {
}

func (*NopRouter) Reconcile(_ *flaggerv1.Canary) error {
	return nil
}

func (*NopRouter) SetRoutes(_ *flaggerv1.Canary, _ int, _ int, _ bool) error {
	return nil
}

func (*NopRouter) GetRoutes(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, mirror bool, err error) {
	if canary.Status.Iterations > 0 {
		return 0, 100, false, nil
	}
	return 100, 0, false, nil
}

func (c *NopRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
