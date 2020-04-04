package canary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestDaemonSetController_Sync(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	dae := newDaemonSetControllerTestPodInfo()
	primaryImage := daePrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dae.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, primaryImage, sourceImage)
}

func TestDaemonSetController_Promote(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	dae2 := newDaemonSetControllerTestPodInfoV2()
	_, err = mocks.kubeClient.AppsV1().DaemonSets("default").Update(context.TODO(), dae2, metav1.UpdateOptions{})
	require.NoError(t, err)

	config2 := newDaemonSetControllerTestConfigMapV2()
	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Update(context.TODO(), config2, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = mocks.controller.Promote(mocks.canary)
	require.NoError(t, err)

	daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	primaryImage := daePrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dae2.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, primaryImage, sourceImage)

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, configPrimary.Data["color"], config2.Data["color"])
	}
}

func TestDaemonSetController_NoConfigTracking(t *testing.T) {
	mocks := newDaemonSetFixture()
	mocks.controller.configTracker = &NopTracker{}

	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
	require.True(t, errors.IsNotFound(err), "Primary ConfigMap shouldn't have been created")

	configName := daePrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
	assert.Equal(t, "podinfo-config-vol", configName)
}

func TestDaemonSetController_HasTargetChanged(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	// save last applied hash
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	err = mocks.controller.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseInitializing})
	require.NoError(t, err)

	// save last promoted hash
	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	err = mocks.controller.SetStatusPhase(canary, flaggerv1.CanaryPhaseInitialized)
	require.NoError(t, err)

	dep, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	depClone := dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(100, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().DaemonSets("default").Update(context.TODO(), depClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// detect change in last applied spec
	isNew, err := mocks.controller.HasTargetChanged(canary)
	require.NoError(t, err)
	assert.True(t, isNew)

	// save hash
	err = mocks.controller.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing})
	require.NoError(t, err)

	dep, err = mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	depClone = dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(1000, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().DaemonSets("default").Update(context.TODO(), depClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// ignore change as hash should be the same with last promoted
	isNew, err = mocks.controller.HasTargetChanged(canary)
	require.NoError(t, err)
	assert.False(t, isNew)

	depClone = dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(600, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().DaemonSets("default").Update(context.TODO(), depClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// detect change
	isNew, err = mocks.controller.HasTargetChanged(canary)
	require.NoError(t, err)
	assert.True(t, isNew)
}

func TestDaemonSetController_Scale(t *testing.T) {
	t.Run("ScaleToZero", func(t *testing.T) {
		mocks := newDaemonSetFixture()
		err := mocks.controller.Initialize(mocks.canary)
		require.NoError(t, err)

		err = mocks.controller.ScaleToZero(mocks.canary)
		require.NoError(t, err)

		c, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		for k := range daemonSetScaleDownNodeSelector {
			_, ok := c.Spec.Template.Spec.NodeSelector[k]
			assert.True(t, ok, "%s should exist in node selector", k)
		}
	})
	t.Run("ScaleFromZeo", func(t *testing.T) {
		mocks := newDaemonSetFixture()
		err := mocks.controller.Initialize(mocks.canary)
		require.NoError(t, err)

		err = mocks.controller.ScaleFromZero(mocks.canary)
		require.NoError(t, err)

		c, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		for k := range daemonSetScaleDownNodeSelector {
			_, ok := c.Spec.Template.Spec.NodeSelector[k]
			assert.False(t, ok, "%s should not exist in node selector", k)
		}
	})
}

func TestDaemonSetController_Finalize(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	err = mocks.controller.Finalize(mocks.canary)
	require.NoError(t, err)

	dep, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	_, ok := dep.Spec.Template.Spec.NodeSelector["flagger.app/scale-to-zero"]
	assert.False(t, ok)
}
