package router

import (
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	smiv1 "github.com/weaveworks/flagger/pkg/apis/smi/v1alpha1"
)

func TestSmiRouter_Sync(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &SmiRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	ts, err := router.smiClient.SplitV1alpha1().TrafficSplits("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	dests := ts.Spec.Backends
	if len(dests) != 2 {
		t.Errorf("Got backends %v wanted %v", len(dests), 2)
	}

	apexName, primaryName, canaryName := canary.GetServiceNames()

	if ts.Spec.Service != apexName {
		t.Errorf("Got service %v wanted %v", ts.Spec.Service, apexName)
	}

	var pRoute smiv1.TrafficSplitBackend
	var cRoute smiv1.TrafficSplitBackend
	for _, dest := range ts.Spec.Backends {
		if dest.Service == primaryName {
			pRoute = dest
		}
		if dest.Service == canaryName {
			cRoute = dest
		}
	}

	if pRoute.Weight.String() != strconv.Itoa(100) {
		t.Errorf("%s weight is %v wanted 100", pRoute.Service, pRoute.Weight)
	}
	if cRoute.Weight.String() != strconv.Itoa(0) {
		t.Errorf("%s weight is %v wanted 0", cRoute.Service, cRoute.Weight)
	}

	// test update
	host := "test"
	canary.Spec.Service.Name = host

	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	ts, err = router.smiClient.SplitV1alpha1().TrafficSplits("default").Get("test", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if ts.Spec.Service != host {
		t.Errorf("Got service %v wanted %v", ts.Spec.Service, host)
	}
}

func TestSmiRouter_SetRoutes(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &SmiRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err := router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p = 50
	c = 50
	m = false

	err = router.SetRoutes(mocks.canary, p, c, m)
	if err != nil {
		t.Fatal(err.Error())
	}

	ts, err := router.smiClient.SplitV1alpha1().TrafficSplits("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	var pRoute smiv1.TrafficSplitBackend
	var cRoute smiv1.TrafficSplitBackend
	_, primaryName, canaryName := canary.GetServiceNames()

	for _, dest := range ts.Spec.Backends {
		if dest.Service == primaryName {
			pRoute = dest
		}
		if dest.Service == canaryName {
			cRoute = dest
		}
	}

	if pRoute.Weight.String() != strconv.Itoa(p) {
		t.Errorf("Got primary weight %v wanted %v", pRoute.Weight, p)
	}

	if cRoute.Weight.String() != strconv.Itoa(c) {
		t.Errorf("Got canary weight %v wanted %v", cRoute.Weight, c)
	}

}

func TestSmiRouter_GetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &SmiRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err := router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 100 {
		t.Errorf("Got primary weight %v wanted %v", p, 100)
	}

	if c != 0 {
		t.Errorf("Got canary weight %v wanted %v", c, 0)
	}

	if m != false {
		t.Errorf("Got mirror %v wanted %v", m, false)
	}
}
