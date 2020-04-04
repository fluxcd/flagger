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
	mocks := newDeploymentFixture()
	mocks.controller.Initialize(mocks.canary)

	err := mocks.controller.IsPrimaryReady(mocks.canary)
	require.Error(t, err)

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	require.NoError(t, err)
}

func TestDeploymentController_isDeploymentReady(t *testing.T) {
	mocks := newDeploymentFixture()

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
