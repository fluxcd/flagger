package canary

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestDeploymentController_Sync(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	dep := newDeploymentControllerTest()

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep.Spec.Template.Spec.Containers[0].Image
	if primaryImage != sourceImage {
		t.Errorf("Got image %s wanted %s", primaryImage, sourceImage)
	}

	hpaPrimary, err := mocks.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if hpaPrimary.Spec.ScaleTargetRef.Name != depPrimary.Name {
		t.Errorf("Got HPA target %s wanted %s", hpaPrimary.Spec.ScaleTargetRef.Name, depPrimary.Name)
	}
}

func TestDeploymentController_Promote(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	dep2 := newDeploymentControllerTestV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	config2 := newDeploymentControllerTestConfigMapV2()
	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Update(config2)
	if err != nil {
		t.Fatal(err.Error())
	}

	hpa, err := mocks.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	hpaClone := hpa.DeepCopy()
	hpaClone.Spec.MaxReplicas = 2

	_, err = mocks.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("default").Update(hpaClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.controller.Promote(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != sourceImage {
		t.Errorf("Got image %s wanted %s", primaryImage, sourceImage)
	}

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimary.Data["color"] != config2.Data["color"] {
		t.Errorf("Got primary ConfigMap color %s wanted %s", configPrimary.Data["color"], config2.Data["color"])
	}

	hpaPrimary, err := mocks.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if hpaPrimary.Spec.MaxReplicas != 2 {
		t.Errorf("Got primary HPA MaxReplicas %v wanted %v", hpaPrimary.Spec.MaxReplicas, 2)
	}
}

func TestDeploymentController_IsReady(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Error("Expected primary readiness check to fail")
	}

	_, err = mocks.controller.IsPrimaryReady(mocks.canary)
	if err == nil {
		t.Fatal(err.Error())
	}

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestDeploymentController_SetFailedChecks(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.controller.SetStatusFailedChecks(mocks.canary, 1)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.FailedChecks != 1 {
		t.Errorf("Got %v wanted %v", res.Status.FailedChecks, 1)
	}
}

func TestDeploymentController_SetState(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.controller.SetStatusPhase(mocks.canary, flaggerv1.CanaryPhaseProgressing)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != flaggerv1.CanaryPhaseProgressing {
		t.Errorf("Got %v wanted %v", res.Status.Phase, flaggerv1.CanaryPhaseProgressing)
	}
}

func TestDeploymentController_SyncStatus(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	status := flaggerv1.CanaryStatus{
		Phase:        flaggerv1.CanaryPhaseProgressing,
		FailedChecks: 2,
	}
	err = mocks.controller.SyncStatus(mocks.canary, status)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != status.Phase {
		t.Errorf("Got state %v wanted %v", res.Status.Phase, status.Phase)
	}

	if res.Status.FailedChecks != status.FailedChecks {
		t.Errorf("Got failed checks %v wanted %v", res.Status.FailedChecks, status.FailedChecks)
	}

	if res.Status.TrackedConfigs == nil {
		t.Fatalf("Status tracking configs are empty")
	}
	configs := *res.Status.TrackedConfigs
	secret := newDeploymentControllerTestSecret()
	if _, exists := configs["secret/"+secret.GetName()]; !exists {
		t.Errorf("Secret %s not found in status", secret.GetName())
	}
}

func TestDeploymentController_Scale(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.controller.Scale(mocks.canary, 2)

	c, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 2 {
		t.Errorf("Got replicas %v wanted %v", *c.Spec.Replicas, 2)
	}
}

func TestDeploymentController_NoConfigTracking(t *testing.T) {
	mocks := newDeploymentFixture()
	mocks.controller.configTracker = &NopTracker{}

	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		t.Fatalf("Primary ConfigMap shouldn't have been created")
	}

	configName := depPrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
	if configName != "podinfo-config-vol" {
		t.Errorf("Got config name %v wanted %v", configName, "podinfo-config-vol")
	}
}

func TestDeploymentController_HasTargetChanged(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// save last applied hash
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	err = mocks.controller.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseInitializing})
	if err != nil {
		t.Fatal(err.Error())
	}

	// save last promoted hash
	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	err = mocks.controller.SetStatusPhase(canary, flaggerv1.CanaryPhaseInitialized)
	if err != nil {
		t.Fatal(err.Error())
	}

	dep, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	depClone := dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(100, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(depClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect change in last applied spec
	isNew, err := mocks.controller.HasTargetChanged(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !isNew {
		t.Errorf("Got %v wanted %v", isNew, true)
	}

	// save hash
	err = mocks.controller.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing})
	if err != nil {
		t.Fatal(err.Error())
	}

	dep, err = mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	depClone = dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(1000, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(depClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// ignore change as hash should be the same with last promoted
	isNew, err = mocks.controller.HasTargetChanged(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
	if isNew {
		t.Errorf("Got %v wanted %v", isNew, false)
	}

	depClone = dep.DeepCopy()
	depClone.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: *resource.NewQuantity(600, resource.DecimalExponent),
		},
	}

	// update pod spec
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(depClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect change
	isNew, err = mocks.controller.HasTargetChanged(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !isNew {
		t.Errorf("Got %v wanted %v", isNew, true)
	}
}
