package router

import (
	"context"
	"fmt"
	"testing"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	solokitmemory "github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	solokitcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	supergloov1 "github.com/solo-io/supergloo/pkg/api/v1"
)

func TestSuperglooRouter_Sync(t *testing.T) {
	mocks := setupfakeClients()

	routingRuleClient, err := supergloov1.NewRoutingRuleClient(&factory.MemoryResourceClientFactory{
		Cache: solokitmemory.NewInMemoryResourceCache(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := routingRuleClient.Register(); err != nil {
		t.Fatal(err.Error())
	}
	targetMesh := solokitcore.ResourceRef{
		Namespace: "supergloo-system",
		Name:      "mesh",
	}
	router := NewSuperglooRouterWithClient(context.TODO(), routingRuleClient, targetMesh, mocks.logger)
	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	rr, err := routingRuleClient.Read("default", "podinfo", solokitclients.ReadOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	dests := rr.Spec.GetTrafficShifting().GetDestinations().GetDestinations()
	if len(dests) != 1 {
		t.Errorf("Got RoutingRule Destinations %v wanted %v", len(dests), 1)
	}

}

func TestSuperglooRouter_SetRoutes(t *testing.T) {

	mocks := setupfakeClients()

	routingRuleClient, err := supergloov1.NewRoutingRuleClient(&factory.MemoryResourceClientFactory{
		Cache: solokitmemory.NewInMemoryResourceCache(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := routingRuleClient.Register(); err != nil {
		t.Fatal(err.Error())
	}
	targetMesh := solokitcore.ResourceRef{
		Namespace: "supergloo-system",
		Name:      "mesh",
	}
	router := NewSuperglooRouterWithClient(context.TODO(), routingRuleClient, targetMesh, mocks.logger)

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

	rr, err := routingRuleClient.Read("default", "podinfo", solokitclients.ReadOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}

	var pRoute *gloov1.WeightedDestination
	var cRoute *gloov1.WeightedDestination
	targetName := mocks.canary.Spec.TargetRef.Name

	for _, dest := range rr.GetSpec().GetTrafficShifting().GetDestinations().GetDestinations() {
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

func TestSuperglooRouter_GetRoutes(t *testing.T) {
	mocks := setupfakeClients()

	routingRuleClient, err := supergloov1.NewRoutingRuleClient(&factory.MemoryResourceClientFactory{
		Cache: solokitmemory.NewInMemoryResourceCache(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := routingRuleClient.Register(); err != nil {
		t.Fatal(err.Error())
	}
	targetMesh := solokitcore.ResourceRef{
		Namespace: "supergloo-system",
		Name:      "mesh",
	}
	router := NewSuperglooRouterWithClient(context.TODO(), routingRuleClient, targetMesh, mocks.logger)
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
