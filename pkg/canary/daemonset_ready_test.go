package canary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestDaemonSetController_IsReady(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	assert.NoError(t, err, "Expected primary readiness check to fail")

	_, err = mocks.controller.IsPrimaryReady(mocks.canary)
	require.NoError(t, err)

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	require.NoError(t, err)
}

func TestDaemonSetController_isDaemonSetReady(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
			UpdatedNumberScheduled: 1,
		},
	}

	cd := &flaggerv1.Canary{}
	cd.Spec.ProgressDeadlineSeconds = int32p(1e5)
	cd.Status.LastTransitionTime = metav1.Now()

	// ready
	mocks := newDaemonSetFixture()
	_, err := mocks.controller.isDaemonSetReady(cd, ds)
	require.NoError(t, err)

	// not ready but retriable
	ds.Status.NumberUnavailable++
	retrieable, err := mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.True(t, retrieable)
	ds.Status.NumberUnavailable--

	ds.Status.DesiredNumberScheduled++
	retrieable, err = mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.True(t, retrieable)

	// not ready and not retriable
	cd.Status.LastTransitionTime = metav1.Now()
	cd.Spec.ProgressDeadlineSeconds = int32p(-1e5)
	retrieable, err = mocks.controller.isDaemonSetReady(cd, ds)
	require.Error(t, err)
	require.False(t, retrieable)

}
