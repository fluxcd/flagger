/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package canary

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestDaemonSetController_Sync_ConsistentNaming(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), fmt.Sprintf("%s-primary", dc.name), metav1.GetOptions{})
	require.NoError(t, err)

	dae := newDaemonSetControllerTestPodInfo(dc)
	primaryImage := daePrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dae.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, primaryImage, sourceImage)

	primarySelectorValue := daePrimary.Spec.Selector.MatchLabels[dc.label]
	sourceSelectorValue := dae.Spec.Selector.MatchLabels[dc.label]
	assert.Equal(t, primarySelectorValue, fmt.Sprintf("%s-primary", sourceSelectorValue))

	annotation := daePrimary.Annotations["kustomize.toolkit.fluxcd.io/checksum"]
	assert.Equal(t, "", annotation)
}

func TestDaemonSetController_Sync_InconsistentNaming(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo-service", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), fmt.Sprintf("%s-primary", dc.name), metav1.GetOptions{})
	require.NoError(t, err)

	dae := newDaemonSetControllerTestPodInfo(dc)
	primaryImage := daePrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dae.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, primaryImage, sourceImage)

	primarySelectorValue := daePrimary.Spec.Selector.MatchLabels[dc.label]
	sourceSelectorValue := dae.Spec.Selector.MatchLabels[dc.label]
	assert.Equal(t, primarySelectorValue, fmt.Sprintf("%s-primary", sourceSelectorValue))
}

func TestDaemonSetController_Promote(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
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

	daePrimaryLabels := daePrimary.ObjectMeta.Labels
	daeSourceLabels := dae2.ObjectMeta.Labels
	assert.Equal(t, daeSourceLabels["app.kubernetes.io/test-label-1"], daePrimaryLabels["app.kubernetes.io/test-label-1"])

	daePrimaryAnnotations := daePrimary.ObjectMeta.Annotations
	daeSourceAnnotations := dae2.ObjectMeta.Annotations
	assert.Equal(t, daeSourceAnnotations["app.kubernetes.io/test-annotation-1"], daePrimaryAnnotations["app.kubernetes.io/test-annotation-1"])

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, configPrimary.Data["color"], config2.Data["color"])
	}
}

func TestDaemonSetController_NoConfigTracking(t *testing.T) {
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
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
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
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
		dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDaemonSetFixture(dc)
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
		dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDaemonSetFixture(dc)
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
	dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDaemonSetFixture(dc)
	err := mocks.controller.Initialize(mocks.canary)
	require.NoError(t, err)

	err = mocks.controller.Finalize(mocks.canary)
	require.NoError(t, err)

	dep, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	_, ok := dep.Spec.Template.Spec.NodeSelector["flagger.app/scale-to-zero"]
	assert.False(t, ok)
}
