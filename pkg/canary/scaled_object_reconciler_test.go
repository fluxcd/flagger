package canary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	keda "github.com/fluxcd/flagger/pkg/apis/keda/v1alpha1"
)

func Test_reconcilePrimaryScaledObject(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "ScaledObject",
	})

	soReconciler := mocks.scalerReconciler.(*ScaledObjectReconciler)

	so, err := mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	err = soReconciler.reconcilePrimaryScaler(mocks.canary, true)
	require.NoError(t, err)

	primarySO, err := mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int(*primarySO.Spec.PollingInterval), 10)
	assert.Equal(t, int(*primarySO.Spec.MinReplicaCount), 1)
	assert.Equal(t, primarySO.Spec.Triggers[0].Metadata["query"], `sum(rate(http_requests_total{deployment="podinfo-primary"}[2m]))`)

	so.Spec.PollingInterval = int32p(20)
	so.Spec.Triggers[0].Metadata["query"] = `sum(rate(http_requests_total{deployment="podinfo-canary"}[10m]))`
	_, err = mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Update(context.TODO(), so, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = soReconciler.reconcilePrimaryScaler(mocks.canary, false)
	require.NoError(t, err)

	primarySO, err = mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int(*primarySO.Spec.PollingInterval), 20)
	assert.Equal(t, primarySO.Spec.Triggers[0].Metadata["query"], `sum(rate(http_requests_total{deployment="podinfo-primary"}[10m]))`)
}

func Test_pauseScaledObject(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "ScaledObject",
	})

	soReconciler := mocks.scalerReconciler.(*ScaledObjectReconciler)
	err := soReconciler.PauseTargetScaler(mocks.canary)
	require.NoError(t, err)

	so, err := mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, so.Annotations[keda.PausedReplicasAnnotation], "0")
}

func Test_resumeScaledObject(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "ScaledObject",
	})

	soReconciler := mocks.scalerReconciler.(*ScaledObjectReconciler)
	err := soReconciler.ResumeTargetScaler(mocks.canary)
	require.NoError(t, err)

	so, err := mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	_, exists := so.Annotations[keda.PausedReplicasAnnotation]
	assert.False(t, exists)
}

func Test_setPrimaryScaledObjectQuery(t *testing.T) {
	cd := &flaggerv1.Canary{
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
				Name: "podinfo",
			},
			AutoscalerRef: &flaggerv1.AutoscalerRefernce{
				Name: "podinfo",
			},
		},
	}
	tests := []struct {
		name      string
		query     string
		wantQuery string
	}{
		{
			name:      "query only has 'podinfo'",
			query:     `sum(rate(http_requests_total{deployment="podinfo"}[2m]))`,
			wantQuery: `sum(rate(http_requests_total{deployment="podinfo-primary"}[2m]))`,
		},
		{
			name:      "query only has 'podinfo-canary'",
			query:     `sum(rate(http_requests_total{deployment="podinfo-canary"}[2m]))`,
			wantQuery: `sum(rate(http_requests_total{deployment="podinfo-primary"}[2m]))`,
		},
		{
			name:      "query has both 'podinfo-canary' and 'podinfo'",
			query:     `sum(rate(http_requests_total{deployment="podinfo-canary", svc="podinfo"}[2m]))`,
			wantQuery: `sum(rate(http_requests_total{deployment="podinfo-primary", svc="podinfo-primary"}[2m]))`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := make(map[string]string)
			metadata["query"] = test.query
			setPrimaryScaledObjectQuery(cd, metadata)
			assert.Equal(t, metadata["query"], test.wantQuery)
		})
	}

	primaryQuery := `sum(rate(envoy_cluster_upstream_rq{ envoy_cluster_name="test_podinfo-primary_80" }[30s]))`
	cd.Spec.AutoscalerRef.PrimaryScalerQuery = primaryQuery
	metadata := make(map[string]string)
	metadata["query"] = ""

	setPrimaryScaledObjectQuery(cd, metadata)
	assert.Equal(t, primaryQuery, metadata["query"])
}
