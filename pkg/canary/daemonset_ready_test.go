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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestDaemonSetController_IsReady(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	err = mocks.controller.IsPrimaryReady(mocks.canary)
	require.NoError(t, err)

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	require.NoError(t, err)
}

func TestDaemonSetController_isDaemonSetReady(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	cd := &flaggerv1.Canary{}

	// observed generation is less than desired generation
	ds := &appsv1.DaemonSet{Status: appsv1.DaemonSetStatus{}}
	ds.Status.ObservedGeneration--
	retryable, err := mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.True(t, retryable)

	// succeeded
	ds = &appsv1.DaemonSet{Status: appsv1.DaemonSetStatus{
		UpdatedNumberScheduled: 1,
		DesiredNumberScheduled: 1,
		NumberAvailable:        1,
	}}
	retryable, err = mocks.controller.isDaemonSetReady(cd, ds)
	require.NoError(t, err)
	require.True(t, retryable)

	// deadline exceeded
	ds = &appsv1.DaemonSet{Status: appsv1.DaemonSetStatus{
		UpdatedNumberScheduled: 0,
		DesiredNumberScheduled: 1,
	}}
	cd.Status.LastTransitionTime = metav1.Now()
	cd.Spec.ProgressDeadlineSeconds = int32p(-1e6)
	retryable, err = mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.False(t, retryable)

	// only newCond not satisfied
	ds = &appsv1.DaemonSet{Status: appsv1.DaemonSetStatus{
		UpdatedNumberScheduled: 0,
		DesiredNumberScheduled: 1,
		NumberAvailable:        1,
	}}
	cd.Spec.ProgressDeadlineSeconds = int32p(1e6)
	retryable, err = mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.True(t, retryable)
	require.True(t, strings.Contains(err.Error(), "new pods"))

	// only availableCond not satisfied
	ds = &appsv1.DaemonSet{Status: appsv1.DaemonSetStatus{
		UpdatedNumberScheduled: 1,
		DesiredNumberScheduled: 1,
		NumberAvailable:        0,
	}}
	retryable, err = mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.True(t, retryable)
	require.True(t, strings.Contains(err.Error(), "available"))
}
