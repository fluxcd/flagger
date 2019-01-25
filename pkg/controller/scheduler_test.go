package controller

import (
	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestScheduler_Init(t *testing.T) {
	mocks := SetupMocks()
	mocks.ctrl.advanceCanary("podinfo", "default", false)

	_, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestScheduler_NewRevision(t *testing.T) {
	mocks := SetupMocks()
	mocks.ctrl.advanceCanary("podinfo", "default", false)

	// update
	dep2 := newTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", false)

	c, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 1 {
		t.Errorf("Got canary replicas %v wanted %v", *c.Spec.Replicas, 1)
	}
}

func TestScheduler_Rollback(t *testing.T) {
	mocks := SetupMocks()
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// update failed checks to max
	err := mocks.deployer.SyncStatus(mocks.canary, v1alpha3.CanaryStatus{Phase: v1alpha3.CanaryProgressing, FailedChecks: 11})
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	c, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != v1alpha3.CanaryFailed {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, v1alpha3.CanaryFailed)
	}
}

func TestScheduler_NewRevisionReset(t *testing.T) {
	mocks := SetupMocks()
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", false)

	// first update
	dep2 := newTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)
	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 90 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 90)
	}

	if canaryRoute.Weight != 10 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 10)
	}

	// second update
	dep2.Spec.Template.Spec.ServiceAccountName = "test"
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err = mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 100)
	}

	if canaryRoute.Weight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 0)
	}
}

func TestScheduler_Promotion(t *testing.T) {
	mocks := SetupMocks()
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", false)

	// update
	dep2 := newTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryRoute.Weight = 60
	canaryRoute.Weight = 40
	err = mocks.ctrl.router.SetRoutes(mocks.canary, primaryRoute, canaryRoute)
	if err != nil {
		t.Fatal(err.Error())
	}

	config2 := NewTestConfigMapV2()
	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Update(config2)
	if err != nil {
		t.Fatal(err.Error())
	}

	secret2 := NewTestSecretV2()
	_, err = mocks.kubeClient.CoreV1().Secrets("default").Update(secret2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// promote
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err = mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 100)
	}

	if canaryRoute.Weight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 0)
	}

	primaryDep, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := primaryDep.Spec.Template.Spec.Containers[0].Image
	canaryImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != canaryImage {
		t.Errorf("Got primary image %v wanted %v", primaryImage, canaryImage)
	}

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimary.Data["color"] != config2.Data["color"] {
		t.Errorf("Got primary ConfigMap color %s wanted %s", configPrimary.Data["color"], config2.Data["color"])
	}

	secretPrimary, err := mocks.kubeClient.CoreV1().Secrets("default").Get("podinfo-secret-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if string(secretPrimary.Data["apiKey"]) != string(secret2.Data["apiKey"]) {
		t.Errorf("Got primary secret %s wanted %s", secretPrimary.Data["apiKey"], secret2.Data["apiKey"])
	}

	c, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != v1alpha3.CanarySucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, v1alpha3.CanarySucceeded)
	}
}
