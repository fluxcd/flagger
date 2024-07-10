package canary

import (
	"context"
	"fmt"
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
	// test that the hpa ownership annotation is added to the primarySO
	assert.Equal(t, primarySO.ObjectMeta.Annotations["scaledobject.keda.sh/transfer-hpa-ownership"], "true")
	// test that the horizontalpodautoscalerconfig is set to 'podinfo-primary', so that it takes over ownership of the HPA
	assert.Equal(t, primarySO.Spec.Advanced.HorizontalPodAutoscalerConfig.Name, "podinfo-primary")
	assert.Equal(t, primarySO.Spec.ScaleTargetRef.Name, fmt.Sprintf("%s-primary", mocks.canary.Spec.TargetRef.Name))
	assert.Equal(t, int(*primarySO.Spec.PollingInterval), 10)
	assert.Equal(t, int(*primarySO.Spec.MinReplicaCount), 1)
	assert.Equal(t, primarySO.Spec.Triggers[0].Metadata["query"], `sum(rate(http_requests_total{app="podinfo-primary"}[2m]))`)

	so.Spec.PollingInterval = int32p(20)
	so.Spec.Triggers[0].Metadata["query"] = `sum(rate(http_requests_total{app="podinfo-canary"}[10m]))`
	_, err = mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Update(context.TODO(), so, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = soReconciler.reconcilePrimaryScaler(mocks.canary, false)
	require.NoError(t, err)

	primarySO, err = mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int(*primarySO.Spec.PollingInterval), 20)
	assert.Equal(t, primarySO.Spec.Triggers[0].Metadata["query"], `sum(rate(http_requests_total{app="podinfo-primary"}[10m]))`)

	// Test reconcile with PrimaryScaledObjectOverride
	mocks.canary.Spec.AutoscalerRef.PrimaryScalerReplicas = &flaggerv1.ScalerReplicas{
		MinReplicas: int32p(2),
		MaxReplicas: int32p(15),
	}
	err = soReconciler.reconcilePrimaryScaler(mocks.canary, false)
	require.NoError(t, err)

	primarySO, err = mocks.flaggerClient.KedaV1alpha1().ScaledObjects("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int(*primarySO.Spec.PollingInterval), 20)
	assert.Equal(t, primarySO.Spec.MinReplicaCount, mocks.canary.Spec.AutoscalerRef.PrimaryScalerReplicas.MinReplicas)
	assert.Equal(t, primarySO.Spec.MaxReplicaCount, mocks.canary.Spec.AutoscalerRef.PrimaryScalerReplicas.MaxReplicas)
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

func Test_setPrimaryScaledObjectQueries(t *testing.T) {
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
			query:     `sum(rate(http_requests_total{app="podinfo"}[2m]))`,
			wantQuery: `sum(rate(http_requests_total{app="podinfo-primary"}[2m]))`,
		},
		{
			name:      "query only has 'podinfo-canary'",
			query:     `sum(rate(http_requests_total{app="podinfo-canary"}[2m]))`,
			wantQuery: `sum(rate(http_requests_total{app="podinfo-primary"}[2m]))`,
		},
		{
			name:      "query has both 'podinfo-canary' and 'podinfo'",
			query:     `sum(rate(http_requests_total{app="podinfo-canary", svc="podinfo"}[2m]))`,
			wantQuery: `sum(rate(http_requests_total{app="podinfo-primary", svc="podinfo-primary"}[2m]))`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			triggers := make([]keda.ScaleTriggers, 0)
			triggers = append(triggers, keda.ScaleTriggers{
				Metadata: map[string]string{
					"query": test.query,
				},
			})
			setPrimaryScaledObjectQueries(cd, triggers)
			assert.Equal(t, triggers[0].Metadata["query"], test.wantQuery)
		})
	}

	pq1 := `sum(rate(envoy_cluster_upstream_rq{ envoy_cluster_name="test_podinfo-primary_80" }[30s]))`
	pq2 := `sum(rate(envoy_cluster_upstream_rq{ envoy_cluster_name="test_podinfo" }[30s]))`
	triggers := make([]keda.ScaleTriggers, 0)
	triggers = append(triggers, keda.ScaleTriggers{
		Name: "trigger1",
		Metadata: map[string]string{
			"query": pq1,
		},
	})
	triggers = append(triggers, keda.ScaleTriggers{
		Name: "trigger2",
		Metadata: map[string]string{
			"query": pq2,
		},
	})
	cd.Spec.AutoscalerRef.PrimaryScalerQueries = map[string]string{
		"trigger1": pq1,
		"trigger2": pq2,
	}

	setPrimaryScaledObjectQueries(cd, triggers)
	for _, trigger := range triggers {
		if trigger.Name == "trigger1" {
			assert.Equal(t, pq1, trigger.Metadata["query"])
		}
		if trigger.Name == "trigger2" {
			assert.Equal(t, pq2, trigger.Metadata["query"])
		}
	}
}
