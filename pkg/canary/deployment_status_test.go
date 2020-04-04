package canary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestDeploymentController_SyncStatus(t *testing.T) {
	mocks := newDeploymentFixture()
	mocks.initializeCanary(t)

	status := flaggerv1.CanaryStatus{
		Phase:        flaggerv1.CanaryPhaseProgressing,
		FailedChecks: 2,
	}
	err := mocks.controller.SyncStatus(mocks.canary, status)
	require.NoError(t, err)

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, status.Phase, res.Status.Phase)
	assert.Equal(t, status.FailedChecks, res.Status.FailedChecks)

	require.NotNil(t, res.Status.TrackedConfigs)
	configs := *res.Status.TrackedConfigs
	secret := newDeploymentControllerTestSecret()
	_, exists := configs["secret/"+secret.GetName()]
	assert.True(t, exists, "Secret %s not found in status", secret.GetName())
}

func TestDeploymentController_SetFailedChecks(t *testing.T) {
	mocks := newDeploymentFixture()
	mocks.initializeCanary(t)

	err := mocks.controller.SetStatusFailedChecks(mocks.canary, 1)
	require.NoError(t, err)

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Status.FailedChecks)
}

func TestDeploymentController_SetState(t *testing.T) {
	mocks := newDeploymentFixture()
	mocks.initializeCanary(t)

	err := mocks.controller.SetStatusPhase(mocks.canary, flaggerv1.CanaryPhaseProgressing)
	require.NoError(t, err)

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseProgressing, res.Status.Phase)
}
