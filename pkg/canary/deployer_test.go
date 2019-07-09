package canary

import (
	"testing"

	"github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCanaryDeployer_Sync(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	dep := newTestDeployment()
	configMap := NewTestConfigMap()
	secret := NewTestSecret()

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

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimary.Data["color"] != configMap.Data["color"] {
		t.Errorf("Got ConfigMap color %s wanted %s", configPrimary.Data["color"], configMap.Data["color"])
	}

	configPrimaryEnv, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-all-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimaryEnv.Data["color"] != configMap.Data["color"] {
		t.Errorf("Got ConfigMap %s wanted %s", configPrimaryEnv.Data["a"], configMap.Data["color"])
	}

	configPrimaryVol, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-vol-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimaryVol.Data["color"] != configMap.Data["color"] {
		t.Errorf("Got ConfigMap color %s wanted %s", configPrimary.Data["color"], configMap.Data["color"])
	}

	secretPrimary, err := mocks.kubeClient.CoreV1().Secrets("default").Get("podinfo-secret-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if string(secretPrimary.Data["apiKey"]) != string(secret.Data["apiKey"]) {
		t.Errorf("Got primary secret %s wanted %s", secretPrimary.Data["apiKey"], secret.Data["apiKey"])
	}

	secretPrimaryEnv, err := mocks.kubeClient.CoreV1().Secrets("default").Get("podinfo-secret-all-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if string(secretPrimaryEnv.Data["apiKey"]) != string(secret.Data["apiKey"]) {
		t.Errorf("Got primary secret %s wanted %s", secretPrimary.Data["apiKey"], secret.Data["apiKey"])
	}

	secretPrimaryVol, err := mocks.kubeClient.CoreV1().Secrets("default").Get("podinfo-secret-vol-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if string(secretPrimaryVol.Data["apiKey"]) != string(secret.Data["apiKey"]) {
		t.Errorf("Got primary secret %s wanted %s", secretPrimary.Data["apiKey"], secret.Data["apiKey"])
	}
}

func TestCanaryDeployer_IsNewSpec(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	dep2 := newTestDeploymentV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	isNew, err := mocks.deployer.HasDeploymentChanged(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !isNew {
		t.Errorf("Got %v wanted %v", isNew, true)
	}
}

func TestCanaryDeployer_Promote(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	dep2 := newTestDeploymentV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	config2 := NewTestConfigMapV2()
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

	err = mocks.deployer.Promote(mocks.canary)
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

func TestCanaryDeployer_IsReady(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Error("Expected primary readiness check to fail")
	}

	_, err = mocks.deployer.IsPrimaryReady(mocks.canary)
	if err == nil {
		t.Fatal(err.Error())
	}

	_, err = mocks.deployer.IsCanaryReady(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestCanaryDeployer_SetFailedChecks(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.deployer.SetStatusFailedChecks(mocks.canary, 1)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.FailedChecks != 1 {
		t.Errorf("Got %v wanted %v", res.Status.FailedChecks, 1)
	}
}

func TestCanaryDeployer_SetState(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.deployer.SetStatusPhase(mocks.canary, v1alpha3.CanaryPhaseProgressing)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != v1alpha3.CanaryPhaseProgressing {
		t.Errorf("Got %v wanted %v", res.Status.Phase, v1alpha3.CanaryPhaseProgressing)
	}
}

func TestCanaryDeployer_SyncStatus(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	status := v1alpha3.CanaryStatus{
		Phase:        v1alpha3.CanaryPhaseProgressing,
		FailedChecks: 2,
	}
	err = mocks.deployer.SyncStatus(mocks.canary, status)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
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
	secret := NewTestSecret()
	if _, exists := configs["secret/"+secret.GetName()]; !exists {
		t.Errorf("Secret %s not found in status", secret.GetName())
	}
}

func TestCanaryDeployer_Scale(t *testing.T) {
	mocks := SetupMocks()
	_, _, err := mocks.deployer.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.deployer.Scale(mocks.canary, 2)

	c, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 2 {
		t.Errorf("Got replicas %v wanted %v", *c.Spec.Replicas, 2)
	}
}
