package router

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestServiceRouter_Create(t *testing.T) {
	mocks := setupfakeClients()
	router := &KubernetesRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if canarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got svc port name %s wanted %s", canarySvc.Spec.Ports[0].Name, "http")
	}

	if canarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got svc port %v wanted %v", canarySvc.Spec.Ports[0].Port, 9898)
	}

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if primarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got primary svc port name %s wanted %s", primarySvc.Spec.Ports[0].Name, "http")
	}

	if primarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got primary svc port %v wanted %v", primarySvc.Spec.Ports[0].Port, 9898)
	}
}

func TestServiceRouter_Update(t *testing.T) {
	mocks := setupfakeClients()
	router := &KubernetesRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	canary, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	canaryClone := canary.DeepCopy()
	canaryClone.Spec.Service.PortName = "grpc"

	c, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Update(canaryClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply changes
	err = router.Reconcile(c)
	if err != nil {
		t.Fatal(err.Error())
	}

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if canarySvc.Spec.Ports[0].Name != "grpc" {
		t.Errorf("Got svc port name %s wanted %s", canarySvc.Spec.Ports[0].Name, "grpc")
	}
}

func TestServiceRouter_Undo(t *testing.T) {
	mocks := setupfakeClients()
	router := &KubernetesRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	svcClone := canarySvc.DeepCopy()
	svcClone.Spec.Ports[0].Name = "http2-podinfo"
	svcClone.Spec.Ports[0].Port = 8080

	_, err = mocks.kubeClient.CoreV1().Services("default").Update(svcClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// undo changes
	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	canarySvc, err = mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if canarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got svc port name %s wanted %s", canarySvc.Spec.Ports[0].Name, "http")
	}

	if canarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got svc port %v wanted %v", canarySvc.Spec.Ports[0].Port, 9898)
	}
}
