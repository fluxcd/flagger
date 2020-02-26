package canary

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigTracker_ConfigMaps(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		mocks := newDeploymentFixture()
		configMap := newDeploymentControllerTestConfigMap()
		configMapProjected := newDeploymentControllerTestConfigProjected()

		err := mocks.controller.Initialize(mocks.canary, true)
		if err != nil {
			t.Fatal(err.Error())
		}

		depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		configPrimaryVolName := depPrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
		if configPrimaryVolName != "podinfo-config-vol-primary" {
			t.Errorf("Got config name %v wanted %v", configPrimaryVolName, "podinfo-config-vol-primary")
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

		configProjectedName := depPrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[0].ConfigMap.Name
		if configProjectedName != "podinfo-config-projected-primary" {
			t.Errorf("Got config name %v wanted %v", configProjectedName, "podinfo-config-projected-primary")
		}

		configPrimaryProjected, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-vol-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if configPrimaryProjected.Data["color"] != configMapProjected.Data["color"] {
			t.Errorf("Got ConfigMap color %s wanted %s", configPrimaryProjected.Data["color"], configMapProjected.Data["color"])
		}
	})

	t.Run("daemonset", func(t *testing.T) {
		mocks := newDaemonSetFixture()
		configMap := newDaemonSetControllerTestConfigMap()
		configMapProjected := newDaemonSetControllerTestConfigProjected()

		err := mocks.controller.Initialize(mocks.canary, true)
		if err != nil {
			t.Fatal(err.Error())
		}

		depPrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get("podinfo-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		configPrimaryVolName := depPrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
		if configPrimaryVolName != "podinfo-config-vol-primary" {
			t.Errorf("Got config name %v wanted %v", configPrimaryVolName, "podinfo-config-vol-primary")
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

		configProjectedName := depPrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[0].ConfigMap.Name
		if configProjectedName != "podinfo-config-projected-primary" {
			t.Errorf("Got config name %v wanted %v", configProjectedName, "podinfo-config-projected-primary")
		}

		configPrimaryProjected, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-vol-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if configPrimaryProjected.Data["color"] != configMapProjected.Data["color"] {
			t.Errorf("Got ConfigMap color %s wanted %s", configPrimaryProjected.Data["color"], configMapProjected.Data["color"])
		}
	})
}

func TestConfigTracker_Secrets(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		mocks := newDeploymentFixture()
		secret := newDeploymentControllerTestSecret()
		secretProjected := newDeploymentControllerTestSecretProjected()

		err := mocks.controller.Initialize(mocks.canary, true)
		if err != nil {
			t.Fatal(err.Error())
		}

		depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		secretPrimaryVolName := depPrimary.Spec.Template.Spec.Volumes[1].VolumeSource.Secret.SecretName
		if secretPrimaryVolName != "podinfo-secret-vol-primary" {
			t.Errorf("Got config name %v wanted %v", secretPrimaryVolName, "podinfo-secret-vol-primary")
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

		secretProjectedName := depPrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[1].Secret.Name
		if secretProjectedName != "podinfo-secret-projected-primary" {
			t.Errorf("Got config name %v wanted %v", secretProjectedName, "podinfo-secret-projected-primary")
		}

		secretPrimaryProjected, err := mocks.kubeClient.CoreV1().Secrets("default").Get("podinfo-secret-projected-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if string(secretPrimaryProjected.Data["apiKey"]) != string(secretProjected.Data["apiKey"]) {
			t.Errorf("Got primary secret %s wanted %s", secretPrimaryProjected.Data["apiKey"], secretProjected.Data["apiKey"])
		}
	})

	t.Run("daemonset", func(t *testing.T) {
		mocks := newDaemonSetFixture()
		secret := newDaemonSetControllerTestSecret()
		secretProjected := newDaemonSetControllerTestSecretProjected()

		err := mocks.controller.Initialize(mocks.canary, true)
		if err != nil {
			t.Fatal(err.Error())
		}

		depPrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get("podinfo-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		secretPrimaryVolName := depPrimary.Spec.Template.Spec.Volumes[1].VolumeSource.Secret.SecretName
		if secretPrimaryVolName != "podinfo-secret-vol-primary" {
			t.Errorf("Got config name %v wanted %v", secretPrimaryVolName, "podinfo-secret-vol-primary")
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

		secretProjectedName := depPrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[1].Secret.Name
		if secretProjectedName != "podinfo-secret-projected-primary" {
			t.Errorf("Got config name %v wanted %v", secretProjectedName, "podinfo-secret-projected-primary")
		}

		secretPrimaryProjected, err := mocks.kubeClient.CoreV1().Secrets("default").Get("podinfo-secret-projected-primary", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		if string(secretPrimaryProjected.Data["apiKey"]) != string(secretProjected.Data["apiKey"]) {
			t.Errorf("Got primary secret %s wanted %s", secretPrimaryProjected.Data["apiKey"], secretProjected.Data["apiKey"])
		}
	})
}
