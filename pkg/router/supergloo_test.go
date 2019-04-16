package router

import (
	"context"
	"testing"

	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	solokitmemory "github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	solokitcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	supergloov1 "github.com/solo-io/supergloo/pkg/api/v1"
)

func TestSupergloo_Sync(t *testing.T) {
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
	// TODO(yuval-k): un hard code this
	targetMesh := solokitcore.ResourceRef{
		Namespace: "supergloo-system",
		Name:      "yuval",
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

	/*
		cd, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		cdClone := cd.DeepCopy()
	*/
}
