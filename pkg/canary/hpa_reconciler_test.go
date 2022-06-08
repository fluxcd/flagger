package canary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hpav2 "k8s.io/api/autoscaling/v2"
	hpav2beta2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_reconcilePrimaryHpa(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
		// avoid creating a v2 HPA.
		excludeObjs: []string{"HPAV2"},
	})
	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)

	err := hpaReconciler.reconcilePrimaryHpa(mocks.canary, true)
	require.NoError(t, err)

	// assert that we fallback to v2beta2, when HPAv2 fails.
	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	hpa, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, hpa)

	mocks = newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
		// avoid creating _any_ HPAs.
		excludeObjs: []string{"HPAV2", "HPAV2Beta2"},
	})
	hpaReconciler = mocks.scalerReconciler.(*HPAReconciler)
	// assert that we return an error if no HPAs are found.
	err = hpaReconciler.reconcilePrimaryHpa(mocks.canary, true)
	require.Error(t, err)
}

func Test_reconcilePrimaryHpaV2(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
	})
	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)

	hpa, err := mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	err = hpaReconciler.reconcilePrimaryHpaV2(mocks.canary, hpa, true)
	require.NoError(t, err)

	primaryHPA, err := mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, primaryHPA.Spec.ScaleTargetRef.Name, "podinfo-primary")
	assert.Equal(t, int(*primaryHPA.Spec.Metrics[0].Resource.Target.AverageUtilization), 99)

	hpa.Spec.Metrics[0].Resource.Target = hpav2.MetricTarget{AverageUtilization: int32p(50)}
	hpa.Spec.MaxReplicas = 10
	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Update(context.TODO(), hpa, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = hpaReconciler.reconcilePrimaryHpaV2(mocks.canary, hpa, false)
	require.NoError(t, err)

	primaryHPA, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int(*primaryHPA.Spec.Metrics[0].Resource.Target.AverageUtilization), 50)
	assert.Equal(t, int(primaryHPA.Spec.MaxReplicas), 10)
}

func Test_reconcilePrimaryHpaV2Beta2(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
	})

	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)

	hpa, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	err = hpaReconciler.reconcilePrimaryHpaV2Beta2(mocks.canary, hpa, true)
	require.NoError(t, err)

	primaryHPA, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, primaryHPA.Spec.ScaleTargetRef.Name, "podinfo-primary")
	assert.Equal(t, int(*primaryHPA.Spec.Metrics[0].Resource.Target.AverageUtilization), 99)

	hpa.Spec.Metrics[0].Resource.Target = hpav2beta2.MetricTarget{AverageUtilization: int32p(50)}
	hpa.Spec.MaxReplicas = 10
	_, err = mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Update(context.TODO(), hpa, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = hpaReconciler.reconcilePrimaryHpaV2Beta2(mocks.canary, hpa, false)
	require.NoError(t, err)

	primaryHPA, err = mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int(*primaryHPA.Spec.Metrics[0].Resource.Target.AverageUtilization), 50)
	assert.Equal(t, int(primaryHPA.Spec.MaxReplicas), 10)
}
