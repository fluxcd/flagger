package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// init
	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// test insert
	rt, err := router.glooClient.GatewayV1().RouteTables("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	dests := rt.Spec.Routes[0].Action.Destination.Destinations
	assert.Len(t, dests, 2)
	assert.Equal(t, uint32(100), dests[0].Weight)
	assert.Equal(t, uint32(0), dests[1].Weight)

	// test headers update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	cdClone.Spec.Analysis.Iterations = 5
	cdClone.Spec.Analysis.Match = newTestABTest().Spec.Analysis.Match
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	rt, err = router.glooClient.GatewayV1().RouteTables("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "x-user-type", rt.Spec.Routes[0].Matchers[0].Headers[0].Name)
	assert.Equal(t, "test", rt.Spec.Routes[0].Matchers[0].Headers[0].Value)
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
	require.NoError(t, err)

	_, _, _, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	p := 50
	c := 50
	m := false

	err = router.SetRoutes(mocks.canary, p, c, m)
	require.NoError(t, err)

	rt, err := router.glooClient.GatewayV1().RouteTables("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	var pRoute gloov1.WeightedDestination
	var cRoute gloov1.WeightedDestination
	canaryName := fmt.Sprintf("%s-%s-canary-%v", mocks.canary.Namespace, mocks.canary.Spec.TargetRef.Name, mocks.canary.Spec.Service.Port)
	primaryName := fmt.Sprintf("%s-%s-primary-%v", mocks.canary.Namespace, mocks.canary.Spec.TargetRef.Name, mocks.canary.Spec.Service.Port)

	for _, dest := range rt.Spec.Routes[0].Action.Destination.Destinations {
		if dest.Destination.Upstream.Name == primaryName {
			pRoute = dest
		}
		if dest.Destination.Upstream.Name == canaryName {
			cRoute = dest
		}
	}

	assert.Equal(t, uint32(p), pRoute.Weight)
	assert.Equal(t, uint32(c), cRoute.Weight)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// test update to A/B
	cdClone := cd.DeepCopy()
	cdClone.Spec.Analysis.Iterations = 5
	cdClone.Spec.Analysis.Match = newTestABTest().Spec.Analysis.Match
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// test set routes for A/B
	err = router.SetRoutes(canary, 0, 100, false)
	require.NoError(t, err)

	rt, err = router.glooClient.GatewayV1().RouteTables("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "x-user-type", rt.Spec.Routes[0].Matchers[0].Headers[0].Name)
	assert.Equal(t, "test", rt.Spec.Routes[0].Matchers[0].Headers[0].Value)
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
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.False(t, m)
}
