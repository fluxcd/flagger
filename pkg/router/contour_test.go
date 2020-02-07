package router

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContourRouter_Reconcile(t *testing.T) {
	mocks := setupfakeClients()
	router := &ContourRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		contourClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// init
	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	proxy, err := router.contourClient.ProjectcontourV1().HTTPProxies("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	services := proxy.Spec.Routes[0].Services
	if len(services) != 2 {
		t.Errorf("Got Services %v wanted %v", len(services), 2)
	}

	if services[0].Weight != 100 {
		t.Errorf("Primary weight should is %v wanted 100", services[0].Weight)
	}
	if services[1].Weight != 0 {
		t.Errorf("Canary weight should is %v wanted 0", services[0].Weight)
	}

	// test update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	cdClone := cd.DeepCopy()
	cdClone.Spec.Service.Port = 8080
	cdClone.Spec.Service.Timeout = "1m"
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cdClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply change
	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	port := proxy.Spec.Routes[0].Services[0].Port
	if port != 8080 {
		t.Errorf("Service port is %v wanted %v", port, 8080)
	}

	timeout := proxy.Spec.Routes[0].TimeoutPolicy.Response
	if timeout != "1m" {
		t.Errorf("HTTPProxy timeout is %v wanted %v", timeout, "1m")
	}

	prefix := proxy.Spec.Routes[0].Conditions[0].Prefix
	if prefix != "/podinfo" {
		t.Errorf("HTTPProxy prefix is %v wanted %v", prefix, "podinfo")
	}

	retry := proxy.Spec.Routes[0].RetryPolicy.NumRetries
	if retry != 10 {
		t.Errorf("HTTPProxy NumRetries is %v wanted %v", retry, 10)
	}

	// test headers update
	cd, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	cdClone = cd.DeepCopy()
	cdClone.Spec.CanaryAnalysis.Iterations = 5
	cdClone.Spec.CanaryAnalysis.Match = newMockABTest().Spec.CanaryAnalysis.Match
	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cdClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply change
	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	header := proxy.Spec.Routes[0].Conditions[0].Header.Exact
	if header != "test" {
		t.Errorf("Route header condition is %v wanted %v", header, "test")
	}
}

func TestContourRouter_Routes(t *testing.T) {
	mocks := setupfakeClients()
	router := &ContourRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		contourClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// init
	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test set routers
	err = router.SetRoutes(mocks.canary, 50, 50, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	proxy, err := router.contourClient.ProjectcontourV1().HTTPProxies("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primary := proxy.Spec.Routes[0].Services[0]
	if primary.Weight != 50 {
		t.Errorf("Got primary weight %v wanted %v", primary.Weight, 50)
	}

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// test get routers
	_, cw, _, err := router.GetRoutes(cd)
	if err != nil {
		t.Fatal(err.Error())
	}

	if cw != 50 {
		t.Errorf("Got canary weight %v wanted %v", cw, 50)
	}

	// test update to A/B
	cdClone := cd.DeepCopy()
	cdClone.Spec.CanaryAnalysis.Iterations = 5
	cdClone.Spec.CanaryAnalysis.Match = newMockABTest().Spec.CanaryAnalysis.Match
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cdClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primary = proxy.Spec.Routes[0].Services[0]
	if primary.Weight != 100 {
		t.Errorf("Got primary weight %v wanted %v", primary.Weight, 100)
	}

	primary = proxy.Spec.Routes[1].Services[0]
	if primary.Weight != 100 {
		t.Errorf("Got primary weight %v wanted %v", primary.Weight, 100)
	}

	// test set routers for A/B
	err = router.SetRoutes(canary, 0, 100, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primary = proxy.Spec.Routes[0].Services[0]
	if primary.Weight != 0 {
		t.Errorf("Got primary weight %v wanted %v", primary.Weight, 0)
	}

	primary = proxy.Spec.Routes[1].Services[0]
	if primary.Weight != 100 {
		t.Errorf("Got primary weight %v wanted %v", primary.Weight, 100)
	}
}
