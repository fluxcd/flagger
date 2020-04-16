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

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// test insert
	ug, err := router.glooClient.GlooV1().UpstreamGroups("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	dests := ug.Spec.Destinations
	assert.Len(t, dests, 2)
	assert.Equal(t, uint32(100), dests[0].Weight)
	assert.Equal(t, uint32(0), dests[1].Weight)
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

	ug, err := router.glooClient.GlooV1().UpstreamGroups("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

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

	assert.Equal(t, uint32(p), pRoute.Weight)
	assert.Equal(t, uint32(c), cRoute.Weight)
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
