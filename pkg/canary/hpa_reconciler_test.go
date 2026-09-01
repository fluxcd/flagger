package canary

import (
	"context"
	"testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hpav2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_reconcilePrimaryHpa(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
	})
	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)

	err := hpaReconciler.reconcilePrimaryHpa(mocks.canary, true)
	require.NoError(t, err)

	mocks = newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
		// avoid creating _any_ HPAs.
		excludeObjs: []string{"HPAV2"},
	})
	hpaReconciler = mocks.scalerReconciler.(*HPAReconciler)
	// assert that we return an error if no HPAs are found.
	err = hpaReconciler.reconcilePrimaryHpa(mocks.canary, true)
	require.Error(t, err)
}

func TestHPAReconciler_ReconcilePrimaryScaler_RemovesManagedHPAWhenAutoscalerRefIsRemoved(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
	})
	mocks.canary.UID = "podinfo-uid"
	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)
	autoscalerRef := *mocks.canary.Spec.AutoscalerRef

	require.NoError(t, hpaReconciler.ReconcilePrimaryScaler(mocks.canary, true))

	primaryHPA, err := mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, metav1.IsControlledBy(primaryHPA, mocks.canary))

	require.NoError(t, mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Delete(context.TODO(), "podinfo", metav1.DeleteOptions{}))
	mocks.canary.Spec.AutoscalerRef = nil
	require.NoError(t, hpaReconciler.ReconcilePrimaryScaler(mocks.canary, true))

	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))

	// Cleanup is idempotent and the primary HPA stays deleted.
	require.NoError(t, hpaReconciler.ReconcilePrimaryScaler(mocks.canary, true))
	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))

	// Restoring the source HPA and autoscalerRef recreates the managed primary HPA.
	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Create(context.TODO(), newScalerReconcilerTestHPAV2(), metav1.CreateOptions{})
	require.NoError(t, err)
	mocks.canary.Spec.AutoscalerRef = &autoscalerRef
	require.NoError(t, hpaReconciler.ReconcilePrimaryScaler(mocks.canary, true))

	primaryHPA, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, metav1.IsControlledBy(primaryHPA, mocks.canary))
}

func TestHPAReconciler_ReconcilePrimaryScaler_PreservesUnmanagedHPA(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
	})
	mocks.canary.UID = "podinfo-uid"
	mocks.canary.Spec.AutoscalerRef = nil
	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)

	unmanagedHPA := newScalerReconcilerTestHPAV2()
	unmanagedHPA.Name = "podinfo-primary"
	unmanagedHPA.Spec.ScaleTargetRef.Name = "podinfo-primary"
	_, err := mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Create(context.TODO(), unmanagedHPA, metav1.CreateOptions{})
	require.NoError(t, err)

	require.NoError(t, hpaReconciler.ReconcilePrimaryScaler(mocks.canary, true))

	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestHPAReconciler_ReconcilePrimaryScaler_PreservesHPAOwnedByAnotherCanary(t *testing.T) {
	mocks := newScalerReconcilerFixture(scalerConfig{
		targetName: "podinfo",
		scaler:     "HorizontalPodAutoscaler",
	})
	mocks.canary.UID = "podinfo-uid"
	mocks.canary.Spec.AutoscalerRef = nil
	hpaReconciler := mocks.scalerReconciler.(*HPAReconciler)

	controller := true
	foreignHPA := newScalerReconcilerTestHPAV2()
	foreignHPA.Name = "podinfo-primary"
	foreignHPA.Spec.ScaleTargetRef.Name = "podinfo-primary"
	foreignHPA.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: flaggerv1.SchemeGroupVersion.String(),
		Kind:       flaggerv1.CanaryKind,
		Name:       "other-canary",
		UID:        types.UID("other-canary-uid"),
		Controller: &controller,
	}}
	_, err := mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Create(context.TODO(), foreignHPA, metav1.CreateOptions{})
	require.NoError(t, err)

	require.NoError(t, hpaReconciler.ReconcilePrimaryScaler(mocks.canary, true))

	_, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
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

	// Test reconcile with PrimaryHorizontalPodAutoscalerOverride
	mocks.canary.Spec.AutoscalerRef.PrimaryScalerReplicas = &flaggerv1.ScalerReplicas{
		MinReplicas: int32p(2),
		MaxReplicas: int32p(15),
	}
	err = hpaReconciler.reconcilePrimaryHpaV2(mocks.canary, hpa, false)
	require.NoError(t, err)

	primaryHPA, err = mocks.kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, primaryHPA.Spec.MinReplicas, mocks.canary.Spec.AutoscalerRef.PrimaryScalerReplicas.MinReplicas)
	assert.Equal(t, primaryHPA.Spec.MaxReplicas, *mocks.canary.Spec.AutoscalerRef.PrimaryScalerReplicas.MaxReplicas)
}
