package router

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gloov1 "github.com/weaveworks/flagger/pkg/apis/gloo/v1"
)

func TestGlooRouter_Sync(t *testing.T) {
	mocks := newFixture(nil)
	router := &GlooRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		glooClient:    mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	ug, err := router.glooClient.GlooV1().UpstreamGroups("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	dests := ug.Spec.Destinations
	if len(dests) != 2 {
		t.Errorf("Got Destinations %v wanted %v", len(dests), 2)
	}

	if dests[0].Weight != 100 {
		t.Errorf("Primary weight should is %v wanted 100", dests[0].Weight)
	}
	if dests[1].Weight != 0 {
		t.Errorf("Canary weight should is %v wanted 0", dests[0].Weight)
	}

}

func TestGlooRouter_SetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &GlooRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		glooClient:    mocks.meshClient,
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

	ug, err := router.glooClient.GlooV1().UpstreamGroups("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	var pRoute gloov1.WeightedDestination
	var cRoute gloov1.WeightedDestination
	canaryName := fmt.Sprintf("%s-%s-canary-%v", mocks.canary.Namespace, mocks.canary.Spec.TargetRef.Name, mocks.canary.Spec.Service.Port)
	primaryName := fmt.Sprintf("%s-%s-primary-%v", mocks.canary.Namespace, mocks.canary.Spec.TargetRef.Name, mocks.canary.Spec.Service.Port)

	for _, dest := range ug.Spec.Destinations {
		if dest.Destination.Upstream.Name == primaryName {
			pRoute = dest
		}
		if dest.Destination.Upstream.Name == canaryName {
			cRoute = dest
		}
	}

	if pRoute.Weight != uint32(p) {
		t.Errorf("Got primary weight %v wanted %v", pRoute.Weight, p)
	}

	if cRoute.Weight != uint32(c) {
		t.Errorf("Got canary weight %v wanted %v", cRoute.Weight, c)
	}

}

func TestGlooRouter_GetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &GlooRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		glooClient:    mocks.meshClient,
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
