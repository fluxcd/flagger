package controller

import (
	"testing"

	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCanaryDeployer_Sync(t *testing.T) {
	canary, kubeClient, _, _, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	dep := newTestDeployment()
	configMap := NewTestConfigMap()

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep.Spec.Template.Spec.Containers[0].Image
	if primaryImage != sourceImage {
		t.Errorf("Got image %s wanted %s", primaryImage, sourceImage)
	}

	hpaPrimary, err := kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if hpaPrimary.Spec.ScaleTargetRef.Name != depPrimary.Name {
		t.Errorf("Got HPA target %s wanted %s", hpaPrimary.Spec.ScaleTargetRef.Name, depPrimary.Name)
	}

	configPrimary, err := kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimary.Data["color"] != configMap.Data["color"] {
		t.Errorf("Got ConfigMap color %s wanted %s", configPrimary.Data["color"], configMap.Data["color"])
	}
}

func TestCanaryDeployer_IsNewSpec(t *testing.T) {
	canary, kubeClient, _, _, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	dep2 := newTestDeploymentUpdated()
	_, err = kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	isNew, err := deployer.IsNewSpec(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !isNew {
		t.Errorf("Got %v wanted %v", isNew, true)
	}
}

func TestCanaryDeployer_Promote(t *testing.T) {
	canary, kubeClient, _, _, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	dep2 := newTestDeploymentUpdated()
	_, err = kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	config2 := NewTestConfigMapUpdated()
	_, err = kubeClient.CoreV1().ConfigMaps("default").Update(config2)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.Promote(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != sourceImage {
		t.Errorf("Got image %s wanted %s", primaryImage, sourceImage)
	}

	configPrimary, err := kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimary.Data["color"] != config2.Data["color"] {
		t.Errorf("Got primary ConfigMap color %s wanted %s", configPrimary.Data["color"], config2.Data["color"])
	}
}

func TestCanaryDeployer_IsReady(t *testing.T) {
	canary, _, _, _, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = deployer.IsPrimaryReady(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = deployer.IsCanaryReady(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestCanaryDeployer_SetFailedChecks(t *testing.T) {
	canary, _, _, flaggerClient, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.SetStatusFailedChecks(canary, 1)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.FailedChecks != 1 {
		t.Errorf("Got %v wanted %v", res.Status.FailedChecks, 1)
	}
}

func TestCanaryDeployer_SetState(t *testing.T) {
	canary, _, _, flaggerClient, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.SetStatusPhase(canary, v1alpha3.CanaryProgressing)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != v1alpha3.CanaryProgressing {
		t.Errorf("Got %v wanted %v", res.Status.Phase, v1alpha3.CanaryProgressing)
	}
}

func TestCanaryDeployer_SyncStatus(t *testing.T) {
	canary, _, _, flaggerClient, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	status := v1alpha3.CanaryStatus{
		Phase:        v1alpha3.CanaryProgressing,
		FailedChecks: 2,
	}
	err = deployer.SyncStatus(canary, status)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != status.Phase {
		t.Errorf("Got state %v wanted %v", res.Status.Phase, status.Phase)
	}

	if res.Status.FailedChecks != status.FailedChecks {
		t.Errorf("Got failed checks %v wanted %v", res.Status.FailedChecks, status.FailedChecks)
	}
}

func TestCanaryDeployer_Scale(t *testing.T) {
	canary, kubeClient, _, _, deployer, _, _, _, _ := SetupTest()

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.Scale(canary, 2)

	c, err := kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 2 {
		t.Errorf("Got replicas %v wanted %v", *c.Spec.Replicas, 2)
	}

}
