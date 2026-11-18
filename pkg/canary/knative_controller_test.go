package canary

import (
	"context"
	"testing"

	"github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKnativeController_Promote(t *testing.T) {
	mocks := newKnativeServiceFixture("podinfo")
	_, err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	service, err := mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	service.Status.LatestCreatedRevisionName = "latest-revision"
	_, err = mocks.knativeClient.ServingV1().Services("default").UpdateStatus(context.TODO(), service, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = mocks.controller.Promote(mocks.canary)
	require.NoError(t, err)

	service, err = mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "latest-revision", service.Annotations["flagger.app/primary-revision"])
}

func TestKnativeController_Initialize(t *testing.T) {
	mocks := newKnativeServiceFixture("podinfo")

	mocks.canary.Status.Phase = v1beta1.CanaryPhasePromoting
	ok, err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, true, ok)

	service, err := mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, service.Annotations, 0)
	assert.Len(t, service.Spec.Traffic, 0)

	mocks.canary.Status.Phase = v1beta1.CanaryPhaseInitializing

	ok, err = mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, true, ok)

	service, err = mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "podinfo-00001", service.Annotations["flagger.app/primary-revision"])
	assert.Len(t, service.Spec.Traffic, 2)
	assert.Equal(t, *service.Spec.Traffic[0].Percent, int64(0))
	assert.True(t, *service.Spec.Traffic[0].LatestRevision)
	assert.Equal(t, *service.Spec.Traffic[1].Percent, int64(100))
	assert.Equal(t, service.Spec.Traffic[1].RevisionName, "podinfo-00001")
}

func TestKnativeController_HasTargetChanged(t *testing.T) {
	mocks := newKnativeServiceFixture("podinfo")
	_, err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	service, err := mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	mocks.canary.Status.Phase = v1beta1.CanaryPhaseSucceeded
	mocks.canary.Status.LastAppliedSpec = ComputeStringHash(service.Status.LatestCreatedRevisionName)
	mocks.canary.Status.LastTrackedRevision = encodeFence(service.UID, service.Status.LatestCreatedRevisionName, "")

	// same revision, no change
	ok, err := mocks.controller.HasTargetChanged(mocks.canary)
	require.NoError(t, err)
	assert.False(t, ok)

	// a new revision was created
	service.Status.LatestCreatedRevisionName = "latest-revision"
	_, err = mocks.knativeClient.ServingV1().Services("default").UpdateStatus(context.TODO(), service, metav1.UpdateOptions{})
	require.NoError(t, err)

	ok, err = mocks.controller.HasTargetChanged(mocks.canary)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestKnativeController_HasTargetChanged_Migration(t *testing.T) {
	// a canary tracked by a previous Flagger version adopts the current
	// revision as baseline when it matches the promoted one
	mocks := newKnativeServiceFixture("podinfo")
	_, err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	mocks.canary.Status.Phase = v1beta1.CanaryPhaseSucceeded
	mocks.canary.Status.LastAppliedSpec = ComputeHash("podinfo-00001") // legacy spew hash
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").UpdateStatus(context.TODO(), mocks.canary, metav1.UpdateOptions{})
	require.NoError(t, err)

	ok, err := mocks.controller.HasTargetChanged(mocks.canary)
	require.NoError(t, err)
	assert.False(t, ok)

	migrated, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Contains(t, migrated.Status.LastAppliedSpec, "v2:")
	assert.NotEmpty(t, migrated.Status.LastTrackedRevision)

	// a revision created while Flagger was not running must trigger instead
	// of being adopted
	stale := newKnativeServiceFixture("podinfo")
	_, err = stale.controller.Initialize(stale.canary)
	require.NoError(t, err)

	service, err := stale.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	service.Status.LatestCreatedRevisionName = "podinfo-00002"
	_, err = stale.knativeClient.ServingV1().Services("default").UpdateStatus(context.TODO(), service, metav1.UpdateOptions{})
	require.NoError(t, err)

	stale.canary.Status.Phase = v1beta1.CanaryPhaseSucceeded
	stale.canary.Status.LastAppliedSpec = ComputeHash("podinfo-00001")

	ok, err = stale.controller.HasTargetChanged(stale.canary)
	require.NoError(t, err)
	assert.True(t, ok)
}
