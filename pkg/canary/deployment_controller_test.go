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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestDeploymentController_Sync_ConsistentNaming(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.initializeCanary(t)

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), fmt.Sprintf("%s-primary", dc.name), metav1.GetOptions{})
	require.NoError(t, err)

	dep := newDeploymentControllerTest(dc)
	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, sourceImage, primaryImage)

	primarySelectorValue := depPrimary.Spec.Selector.MatchLabels[dc.label]
	assert.Equal(t, primarySelectorValue, fmt.Sprintf("%s-primary", dc.labelValue))

	annotation := depPrimary.Annotations["kustomize.toolkit.fluxcd.io/checksum"]
	assert.Equal(t, "", annotation)

	hpaPrimary, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, depPrimary.Name, hpaPrimary.Spec.ScaleTargetRef.Name)
}

func TestDeploymentController_Sync_InconsistentNaming(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo-service", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.initializeCanary(t)

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), fmt.Sprintf("%s-primary", dc.name), metav1.GetOptions{})
	require.NoError(t, err)

	dep := newDeploymentControllerTest(dc)
	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, sourceImage, primaryImage)

	primarySelectorValue := depPrimary.Spec.Selector.MatchLabels[dc.label]
	assert.Equal(t, primarySelectorValue, fmt.Sprintf("%s-primary", dc.labelValue))

	hpaPrimary, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, depPrimary.Name, hpaPrimary.Spec.ScaleTargetRef.Name)
}

func TestDeploymentController_Promote(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.initializeCanary(t)

	dep2 := newDeploymentControllerTestV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	config2 := newDeploymentControllerTestConfigMapV2()
	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Update(context.TODO(), config2, metav1.UpdateOptions{})
	require.NoError(t, err)

	hpa, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	hpaClone := hpa.DeepCopy()
	hpaClone.Spec.MaxReplicas = 2

	_, err = mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Update(context.TODO(), hpaClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = mocks.controller.Promote(mocks.canary)
	require.NoError(t, err)

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep2.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, sourceImage, primaryImage)

	depPrimaryLabels := depPrimary.ObjectMeta.Labels
	depSourceLabels := dep2.ObjectMeta.Labels
	assert.Equal(t, depSourceLabels["app.kubernetes.io/test-label-1"], depPrimaryLabels["app.kubernetes.io/test-label-1"])

	depPrimaryAnnotations := depPrimary.ObjectMeta.Annotations
	depSourceAnnotations := dep2.ObjectMeta.Annotations
	assert.Equal(t, depSourceAnnotations["app.kubernetes.io/test-annotation-1"], depPrimaryAnnotations["app.kubernetes.io/test-annotation-1"])

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, config2.Data["color"], configPrimary.Data["color"])

	hpaPrimary, err := mocks.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int32(2), hpaPrimary.Spec.MaxReplicas)

	hpaPrimaryLabels := hpaPrimary.ObjectMeta.Labels
	hpaSourceLabels := hpa.ObjectMeta.Labels
	assert.Equal(t, hpaSourceLabels["app.kubernetes.io/test-label-1"], hpaPrimaryLabels["app.kubernetes.io/test-label-1"])

	hpaPrimaryAnnotations := hpaPrimary.ObjectMeta.Annotations
	hpaSourceAnnotations := hpa.ObjectMeta.Annotations
	assert.Equal(t, hpaSourceAnnotations["app.kubernetes.io/test-annotation-1"], hpaPrimaryAnnotations["app.kubernetes.io/test-annotation-1"])

	value := depPrimary.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].PodAffinityTerm.LabelSelector.MatchExpressions[0].Values[0]
	assert.Equal(t, "podinfo-primary", value)

	value = depPrimary.Spec.Template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchExpressions[0].Values[0]
	assert.Equal(t, "podinfo-primary", value)
}

func TestDeploymentController_ScaleToZero(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.initializeCanary(t)

	err := mocks.controller.ScaleToZero(mocks.canary)
	require.NoError(t, err)

	c, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), *c.Spec.Replicas)
}

func TestDeploymentController_NoConfigTracking(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.controller.configTracker = &NopTracker{}
	mocks.initializeCanary(t)

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
	require.True(t, errors.IsNotFound(err), "Primary ConfigMap shouldn't have been created")

	configName := depPrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
	assert.Equal(t, "podinfo-config-vol", configName)
}

func TestDeploymentController_HasTargetChanged(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)
	mocks.initializeCanary(t)

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

	dep, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	depClone := dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(100, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), depClone, metav1.UpdateOptions{})
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

	dep, err = mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	depClone = dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(1000, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), depClone, metav1.UpdateOptions{})
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
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), depClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// detect change
	isNew, err = mocks.controller.HasTargetChanged(canary)
	require.NoError(t, err)
	assert.True(t, isNew)
}

func TestDeploymentController_Finalize(t *testing.T) {
	dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
	mocks := newDeploymentFixture(dc)

	for _, tc := range []struct {
		mocks          deploymentControllerFixture
		callInitialize bool
		canary         *flaggerv1.Canary
	}{
		// primary not found returns error
		{mocks, false, mocks.canary},
		// happy path
		{mocks, true, mocks.canary},
	} {
		if tc.callInitialize {
			mocks.initializeCanary(t)
		}

		err := mocks.controller.Finalize(tc.canary)
		require.NoError(t, err)

		c, err := mocks.kubeClient.AppsV1().Deployments(mocks.canary.Namespace).Get(context.TODO(), mocks.canary.Name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, int32(1), *c.Spec.Replicas)
	}
}

func TestDeploymentController_AntiAffinityAndTopologySpreadConstraints(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDeploymentFixture(dc)
		mocks.initializeCanary(t)

		depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
		require.NoError(t, err)

		spec := depPrimary.Spec.Template.Spec

		preferredConstraints := spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		value := preferredConstraints[0].PodAffinityTerm.LabelSelector.MatchExpressions[0].Values[0]
		assert.Equal(t, "podinfo-primary", value)
		value = preferredConstraints[1].PodAffinityTerm.LabelSelector.MatchExpressions[0].Values[0]
		assert.False(t, strings.HasSuffix(value, "-primary"))

		requiredConstraints := spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		value = requiredConstraints[0].LabelSelector.MatchExpressions[0].Values[0]
		assert.Equal(t, "podinfo-primary", value)
		value = requiredConstraints[1].LabelSelector.MatchExpressions[0].Values[0]
		assert.False(t, strings.HasSuffix(value, "-primary"))

		topologySpreadConstraints := spec.TopologySpreadConstraints
		value = topologySpreadConstraints[0].LabelSelector.MatchExpressions[0].Values[0]
		assert.Equal(t, "podinfo-primary", value)
		value = topologySpreadConstraints[1].LabelSelector.MatchExpressions[0].Values[0]
		assert.False(t, strings.HasSuffix(value, "-primary"))
	})
}
