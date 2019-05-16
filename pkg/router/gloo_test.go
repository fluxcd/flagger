package router

import (
	"context"
	"fmt"
	"testing"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	solokitmemory "github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
)

func TestGlooRouter_Sync(t *testing.T) {
	mocks := setupfakeClients()

	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(&factory.MemoryResourceClientFactory{
		Cache: solokitmemory.NewInMemoryResourceCache(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := upstreamGroupClient.Register(); err != nil {
		t.Fatal(err.Error())
	}
	router := NewGlooRouterWithClient(context.TODO(), upstreamGroupClient, "gloo-system", mocks.logger)
	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	ug, err := upstreamGroupClient.Read("default", "podinfo", solokitclients.ReadOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	dests := ug.GetDestinations()
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

	mocks := setupfakeClients()

	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(&factory.MemoryResourceClientFactory{
		Cache: solokitmemory.NewInMemoryResourceCache(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := upstreamGroupClient.Register(); err != nil {
		t.Fatal(err.Error())
	}
	router := NewGlooRouterWithClient(context.TODO(), upstreamGroupClient, "gloo-system", mocks.logger)

	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p = 50
	c = 50

	err = router.SetRoutes(mocks.canary, p, c)
	if err != nil {
		t.Fatal(err.Error())
	}

	ug, err := upstreamGroupClient.Read("default", "podinfo", solokitclients.ReadOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}

	var pRoute *gloov1.WeightedDestination
	var cRoute *gloov1.WeightedDestination
	targetName := mocks.canary.Spec.TargetRef.Name

	for _, dest := range ug.GetDestinations() {
		if dest.GetDestination().GetUpstream().Name == upstreamName(mocks.canary.Namespace, fmt.Sprintf("%s-primary", targetName), mocks.canary.Spec.Service.Port) {
			pRoute = dest
		}
		if dest.GetDestination().GetUpstream().Name == upstreamName(mocks.canary.Namespace, fmt.Sprintf("%s-canary", targetName), mocks.canary.Spec.Service.Port) {
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
	mocks := setupfakeClients()

	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(&factory.MemoryResourceClientFactory{
		Cache: solokitmemory.NewInMemoryResourceCache(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := upstreamGroupClient.Register(); err != nil {
		t.Fatal(err.Error())
	}
	router := NewGlooRouterWithClient(context.TODO(), upstreamGroupClient, "gloo-system", mocks.logger)
	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 100 {
		t.Errorf("Got primary weight %v wanted %v", p, 100)
	}

	if c != 0 {
		t.Errorf("Got canary weight %v wanted %v", c, 0)
	}
}
