package controller

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestScheduler_Init(t *testing.T) {
	mocks := SetupMocks(nil)
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	_, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestScheduler_NewRevision(t *testing.T) {
	mocks := SetupMocks(nil)
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// update
	dep2 := newTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	c, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 1 {
		t.Errorf("Got canary replicas %v wanted %v", *c.Spec.Replicas, 1)
	}
}

func TestScheduler_Rollback(t *testing.T) {
	mocks := SetupMocks(nil)
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// update failed checks to max
	err := mocks.deployer.SyncStatus(mocks.canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing, FailedChecks: 11})
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseFailed {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseFailed)
	}
}

func TestScheduler_SkipAnalysis(t *testing.T) {
	mocks := SetupMocks(nil)
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// enable skip
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cd.Spec.SkipAnalysis = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cd)
	if err != nil {
		t.Fatal(err.Error())
	}

	// update
	dep2 := newTestDeploymentV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)
	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !c.Spec.SkipAnalysis {
		t.Errorf("Got skip analysis %v wanted %v", c.Spec.SkipAnalysis, true)
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseSucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseSucceeded)
	}
}

func TestScheduler_NewRevisionReset(t *testing.T) {
	mocks := SetupMocks(nil)
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

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

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 90 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 90)
	}

	if canaryWeight != 10 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 10)
	}

	if mirrored != false {
		t.Errorf("Got mirrored %v wanted %v", mirrored, false)
	}

	// second update
	dep2.Spec.Template.Spec.ServiceAccountName = "test"
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 100)
	}

	if canaryWeight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 0)
	}

	if mirrored != false {
		t.Errorf("Got mirrored %v wanted %v", mirrored, false)
	}
}

func TestScheduler_Promotion(t *testing.T) {
	mocks := SetupMocks(nil)

	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check initialized status
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseInitialized {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseInitialized)
	}

	// update
	dep2 := newTestDeploymentV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect pod spec changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

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

	// detect configs changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryWeight = 60
	canaryWeight = 40
	err = mocks.router.SetRoutes(mocks.canary, primaryWeight, canaryWeight, mirrored)
	if err != nil {
		t.Fatal(err.Error())
	}

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check progressing status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseProgressing {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseProgressing)
	}

	// promote
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check promoting status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhasePromoting {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhasePromoting)
	}

	// finalise
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 100)
	}

	if canaryWeight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 0)
	}

	if mirrored != false {
		t.Errorf("Got mirrored %v wanted %v", mirrored, false)
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

	// check finalising status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseFinalising {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseFinalising)
	}

	// scale canary to zero
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseSucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseSucceeded)
	}
}

func TestScheduler_Mirroring(t *testing.T) {
	mocks := SetupMocks(newTestCanaryMirror())
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// update
	dep2 := newTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect pod spec changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check if traffic is mirrored to canary
	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 100)
	}

	if canaryWeight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 0)
	}

	if mirrored != true {
		t.Errorf("Got mirrored %v wanted %v", mirrored, true)
	}

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check if traffic is mirrored to canary
	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 90 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 90)
	}

	if canaryWeight != 10 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 10)
	}

	if mirrored != false {
		t.Errorf("Got mirrored %v wanted %v", mirrored, false)
	}
}

func TestScheduler_ABTesting(t *testing.T) {
	mocks := SetupMocks(newTestCanaryAB())
	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// update
	dep2 := newTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect pod spec changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check if traffic is routed to canary
	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 0 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 0)
	}

	if canaryWeight != 100 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 100)
	}

	if mirrored != false {
		t.Errorf("Got mirrored %v wanted %v", mirrored, false)
	}

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// set max iterations
	if err := mocks.deployer.SetStatusIterations(cd, 10); err != nil {
		t.Fatal(err.Error())
	}

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// finalising
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check finalising status
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseFinalising {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseFinalising)
	}

	// check if the container image tag was updated
	primaryDep, err := mocks.kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := primaryDep.Spec.Template.Spec.Containers[0].Image
	canaryImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != canaryImage {
		t.Errorf("Got primary image %v wanted %v", primaryImage, canaryImage)
	}

	// shutdown canary
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check rollout status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseSucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseSucceeded)
	}
}

func TestScheduler_PortDiscovery(t *testing.T) {
	mocks := SetupMocks(nil)

	// enable port discovery
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cd.Spec.Service.PortDiscovery = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cd)
	if err != nil {
		t.Fatal(err.Error())
	}

	mocks.ctrl.advanceCanary("podinfo", "default", true)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(canarySvc.Spec.Ports) != 3 {
		t.Fatalf("Got svc port count %v wanted %v", len(canarySvc.Spec.Ports), 3)
	}

	matchPorts := func(lookup string) bool {
		switch lookup {
		case
			"http 9898",
			"http-metrics 8080",
			"tcp-podinfo-2 8888":
			return true
		}
		return false
	}

	for _, port := range canarySvc.Spec.Ports {
		if !matchPorts(fmt.Sprintf("%s %v", port.Name, port.Port)) {
			t.Fatalf("Got wrong svc port %v", port.Name)
		}

	}
}

func TestScheduler_TargetPortNumber(t *testing.T) {
	mocks := SetupMocks(nil)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cd.Spec.Service.Port = 80
	cd.Spec.Service.TargetPort = intstr.FromInt(9898)
	cd.Spec.Service.PortDiscovery = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cd)
	if err != nil {
		t.Fatal(err.Error())
	}

	mocks.ctrl.advanceCanary("podinfo", "default", true)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(canarySvc.Spec.Ports) != 3 {
		t.Fatalf("Got svc port count %v wanted %v", len(canarySvc.Spec.Ports), 3)
	}

	matchPorts := func(lookup string) bool {
		switch lookup {
		case
			"http 80",
			"http-metrics 8080",
			"tcp-podinfo-2 8888":
			return true
		}
		return false
	}

	for _, port := range canarySvc.Spec.Ports {
		if !matchPorts(fmt.Sprintf("%s %v", port.Name, port.Port)) {
			t.Fatalf("Got wrong svc port %v", port.Name)
		}

	}
}

func TestScheduler_TargetPortName(t *testing.T) {
	mocks := SetupMocks(nil)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	cd.Spec.Service.Port = 8080
	cd.Spec.Service.TargetPort = intstr.FromString("http")
	cd.Spec.Service.PortDiscovery = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cd)
	if err != nil {
		t.Fatal(err.Error())
	}

	mocks.ctrl.advanceCanary("podinfo", "default", true)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(canarySvc.Spec.Ports) != 3 {
		t.Fatalf("Got svc port count %v wanted %v", len(canarySvc.Spec.Ports), 3)
	}

	matchPorts := func(lookup string) bool {
		switch lookup {
		case
			"http 8080",
			"http-metrics 8080",
			"tcp-podinfo-2 8888":
			return true
		}
		return false
	}

	for _, port := range canarySvc.Spec.Ports {
		if !matchPorts(fmt.Sprintf("%s %v", port.Name, port.Port)) {
			t.Fatalf("Got wrong svc port %v", port.Name)
		}

	}
}
