package router

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	serving "knative.dev/serving/pkg/apis/serving/v1"
)

func TestKnativeRouter_Reconcile(t *testing.T) {
	canary := newTestKnativeCanary()
	mocks := newFixture(canary)

	router := &KnativeRouter{
		knativeClient: mocks.knativeClient,
		logger:        mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	service, err := mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "podinfo-00001", service.Annotations["flagger.app/primary-revision"])
}

func TestKnativeRouter_SetRoutes(t *testing.T) {
	canary := newTestKnativeCanary()
	mocks := newFixture(canary)

	router := &KnativeRouter{
		knativeClient: mocks.knativeClient,
		logger:        mocks.logger,
	}

	// error when annotation is not set
	err := router.SetRoutes(canary, 10, 90, false)
	require.Error(t, err)

	err = router.Reconcile(canary)
	require.NoError(t, err)
	err = router.SetRoutes(canary, 10, 90, false)
	require.NoError(t, err)

	service, err := mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, service.Spec.Traffic, 2)
	assert.Equal(t, *service.Spec.Traffic[0].LatestRevision, true)
	assert.Equal(t, *service.Spec.Traffic[0].Percent, int64(90))
	assert.Equal(t, service.Spec.Traffic[1].RevisionName, "podinfo-00001")
	assert.Equal(t, *service.Spec.Traffic[1].Percent, int64(10))
}

func TestKnativeRouter_GetRoutes(t *testing.T) {
	canary := newTestKnativeCanary()
	mocks := newFixture(canary)

	router := &KnativeRouter{
		knativeClient: mocks.knativeClient,
		logger:        mocks.logger,
	}

	// error when annotation is not set
	_, _, _, err := router.GetRoutes(canary)
	require.Error(t, err)

	err = router.Reconcile(canary)
	require.NoError(t, err)

	service, err := mocks.knativeClient.ServingV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	canaryPercent := int64(90)
	primaryPercent := int64(10)
	latestRevision := true
	service.Status.Traffic = []serving.TrafficTarget{
		{
			LatestRevision: &latestRevision,
			Percent:        &canaryPercent,
		},
		{
			RevisionName: "podinfo-00001",
			Percent:      &primaryPercent,
		},
	}
	_, err = mocks.knativeClient.ServingV1().Services("default").Update(context.TODO(), service, metav1.UpdateOptions{})
	require.NoError(t, err)

	pWeight, cWeight, _, err := router.GetRoutes(canary)
	require.NoError(t, err)
	assert.Equal(t, pWeight, 10)
	assert.Equal(t, cWeight, 90)
}
