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

	mocks.canary.Status.LastAppliedSpec = ComputeHash(service.Status.LatestCreatedRevisionName)

	service.Status.LatestCreatedRevisionName = "latest-revision"
	_, err = mocks.knativeClient.ServingV1().Services("default").UpdateStatus(context.TODO(), service, metav1.UpdateOptions{})
	require.NoError(t, err)

	ok, err := mocks.controller.HasTargetChanged(mocks.canary)
	require.NoError(t, err)
	assert.True(t, ok)
}
