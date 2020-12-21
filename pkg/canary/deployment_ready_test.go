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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeploymentController_IsReady(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.controller.Initialize(mocks.canary)

	err := mocks.controller.IsPrimaryReady(mocks.canary)
	require.Error(t, err)

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	require.NoError(t, err)
}

func TestDeploymentController_isDeploymentReady(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)

	// observed generation is less than desired generation
	dp := &appsv1.Deployment{Status: appsv1.DeploymentStatus{ObservedGeneration: -1}}
	retryable, err := mocks.controller.isDeploymentReady(dp, 0)
	assert.Error(t, err)
	assert.True(t, retryable)
	assert.True(t, strings.Contains(err.Error(), "generation"))

	// ok
	dp = &appsv1.Deployment{Status: appsv1.DeploymentStatus{
		Replicas:          1,
		UpdatedReplicas:   1,
		ReadyReplicas:     1,
		AvailableReplicas: 1,
	}}
	retryable, err = mocks.controller.isDeploymentReady(dp, 0)
	assert.NoError(t, err)
	assert.True(t, retryable)

	// old
	dp = &appsv1.Deployment{Status: appsv1.DeploymentStatus{
		UpdatedReplicas: 1,
	}, Spec: appsv1.DeploymentSpec{
		Replicas: int32p(2),
	}}
	_, err = mocks.controller.isDeploymentReady(dp, 0)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "new replicas"))

	// waiting for old replicas to be terminated
	dp = &appsv1.Deployment{Status: appsv1.DeploymentStatus{
		UpdatedReplicas: 1,
		Replicas:        2,
	}}
	_, err = mocks.controller.isDeploymentReady(dp, 0)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "termination"))

	// waiting for updated ones to be available
	dp = &appsv1.Deployment{Status: appsv1.DeploymentStatus{
		UpdatedReplicas:   2,
		AvailableReplicas: 1,
	}}
	_, err = mocks.controller.isDeploymentReady(dp, 0)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "available"))

	// ProgressDeadlineExceeded
	dp = &appsv1.Deployment{Status: appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded"}},
	}}
	retryable, err = mocks.controller.isDeploymentReady(dp, 0)
	assert.Error(t, err)
	assert.False(t, retryable)
	assert.True(t, strings.Contains(err.Error(), "exceeded"))

	// deadline exceeded
	dp = &appsv1.Deployment{Status: appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentProgressing},
			{
				Type:           appsv1.DeploymentAvailable,
				Status:         "False",
				Reason:         "MinimumReplicasUnavailable",
				LastUpdateTime: v1.NewTime(time.Now().Add(-10 * time.Second)),
			},
		},
		UpdatedReplicas:   2,
		AvailableReplicas: 1,
	}}
	retryable, err = mocks.controller.isDeploymentReady(dp, 5)
	assert.Error(t, err)
	assert.False(t, retryable)
}
