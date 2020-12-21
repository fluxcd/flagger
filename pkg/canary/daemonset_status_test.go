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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestDaemonSetController_SyncStatus(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	status := flaggerv1.CanaryStatus{
		Phase:        flaggerv1.CanaryPhaseProgressing,
		FailedChecks: 2,
	}
	err = mocks.controller.SyncStatus(mocks.canary, status)
	require.NoError(t, err)

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, status.Phase, res.Status.Phase)
	assert.Equal(t, status.FailedChecks, res.Status.FailedChecks)
	require.NotNil(t, res.Status.TrackedConfigs)

	configs := *res.Status.TrackedConfigs
	secret := newDaemonSetControllerTestSecret()
	_, exists := configs["secret/"+secret.GetName()]
	assert.True(t, exists, "Secret %s not found in status", secret.GetName())
}

func TestDaemonSetController_SetFailedChecks(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	err = mocks.controller.SetStatusFailedChecks(mocks.canary, 1)
	require.NoError(t, err)

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Status.FailedChecks)
}

func TestDaemonSetController_SetState(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	err = mocks.controller.SetStatusPhase(mocks.canary, flaggerv1.CanaryPhaseProgressing)
	require.NoError(t, err)

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseProgressing, res.Status.Phase)
}
