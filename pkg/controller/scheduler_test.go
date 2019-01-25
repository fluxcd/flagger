package controller

import (
	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestScheduler_Init(t *testing.T) {
	_, kubeClient, _, _, _, _, _, ctrl, _ := SetupTest()

	ctrl.advanceCanary("podinfo", "default", false)

	_, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestScheduler_NewRevision(t *testing.T) {
	_, kubeClient, _, _, _, _, _, ctrl, _ := SetupTest()

	// init
	ctrl.advanceCanary("podinfo", "default", false)

	// update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", false)

	c, err := kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 1 {
		t.Errorf("Got canary replicas %v wanted %v", *c.Spec.Replicas, 1)
	}
}

func TestScheduler_Rollback(t *testing.T) {
	canary, _, _, flaggerClient, deployer, _, _, ctrl, _ := SetupTest()

	// init
	ctrl.advanceCanary("podinfo", "default", true)

	// update failed checks to max
	err := deployer.SyncStatus(canary, v1alpha3.CanaryStatus{Phase: v1alpha3.CanaryProgressing, FailedChecks: 11})
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)

	c, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != v1alpha3.CanaryFailed {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, v1alpha3.CanaryFailed)
	}
}

func TestScheduler_NewRevisionReset(t *testing.T) {
	canary, kubeClient, _, _, _, router, _, ctrl, _ := SetupTest()

	// init
	ctrl.advanceCanary("podinfo", "default", false)

	// first update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)
	// advance
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err := router.GetRoutes(canary)
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
	_, err = kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err = router.GetRoutes(canary)
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
	canary, kubeClient, _, flaggerClient, _, router, _, ctrl, _ := SetupTest()

	// init
	ctrl.advanceCanary("podinfo", "default", false)

	// update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err := router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryRoute.Weight = 60
	canaryRoute.Weight = 40
	err = ctrl.router.SetRoutes(canary, primaryRoute, canaryRoute)
	if err != nil {
		t.Fatal(err.Error())
	}

	config2 := NewTestConfigMapUpdated()
	_, err = kubeClient.CoreV1().ConfigMaps("default").Update(config2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// advance
	ctrl.advanceCanary("podinfo", "default", true)

	// promote
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err = router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 100)
	}

	if canaryRoute.Weight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 0)
	}

	primaryDep, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := primaryDep.Spec.Template.Spec.Containers[0].Image
	canaryImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != canaryImage {
		t.Errorf("Got primary image %v wanted %v", primaryImage, canaryImage)
	}

	configPrimary, err := kubeClient.CoreV1().ConfigMaps("default").Get("podinfo-config-env-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if configPrimary.Data["color"] != config2.Data["color"] {
		t.Errorf("Got primary ConfigMap color %s wanted %s", configPrimary.Data["color"], config2.Data["color"])
	}

	c, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != v1alpha3.CanarySucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, v1alpha3.CanarySucceeded)
	}
}
