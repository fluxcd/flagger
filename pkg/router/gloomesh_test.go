/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package router

import (
	"context"
	"testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	v2 "github.com/fluxcd/flagger/pkg/apis/gloo/networking/v2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	testRouteTableRef = &flaggerv1.CrossNamespaceObjectReference{
		Name:      "podinfo",
		Namespace: "default",
	}
)

func TestMissingRouteTableRef_Throws_Error(t *testing.T) {
	mocks := newFixture(nil)

	router := &GlooMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kubeClient:    mocks.kubeClient,
	}
	svcRouter := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := svcRouter.Initialize(mocks.canary)
	require.NoError(t, err)
	err = svcRouter.Reconcile(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	assert.Error(t, err)
}

func TestGlooMeshRouter_Sync(t *testing.T) {
	mocks := newFixture(nil)

	router := &GlooMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kubeClient:    mocks.kubeClient,
	}
	svcRouter := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	initializeAndReconcileCanaryServices(t, mocks.canary, svcRouter, router)

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	rt, err := router.flaggerClient.GloomeshnetworkingV2().RouteTables("default").
		Get(context.TODO(), "podinfo-delegate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotNil(t, rt, "Delegate Route Created")
	weightedRoute := getWeightedRoute(rt.Spec.Http)

	assert.Len(t, weightedRoute.ForwardTo.Destinations, 2)
	assert.Equal(t, uint32(100), weightedRoute.ForwardTo.Destinations[0].Weight)
	assert.Equal(t, uint32(0), weightedRoute.ForwardTo.Destinations[1].Weight)
}

func TestGlooMeshRouter_SetRoutes(t *testing.T) {
	mocks := newFixture(nil)

	router := &GlooMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kubeClient:    mocks.kubeClient,
	}
	svcRouter := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	initializeAndReconcileCanaryServices(t, mocks.canary, svcRouter, router)

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	err = router.SetRoutes(mocks.canary, 90, 10, false)
	require.NoError(t, err)

	rt, err := router.flaggerClient.GloomeshnetworkingV2().RouteTables("default").
		Get(context.TODO(), "podinfo-delegate", metav1.GetOptions{})
	require.NoError(t, err)
	weightedRoute := getWeightedRoute(rt.Spec.Http)

	assert.Len(t, weightedRoute.ForwardTo.Destinations, 2)
	assert.Equal(t, uint32(90), weightedRoute.ForwardTo.Destinations[0].Weight)
	assert.Equal(t, uint32(10), weightedRoute.ForwardTo.Destinations[1].Weight)
}

func TestGlooMeshRouter_GetRoutes(t *testing.T) {
	mocks := newFixture(nil)

	router := &GlooMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kubeClient:    mocks.kubeClient,
	}
	svcRouter := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	initializeAndReconcileCanaryServices(t, mocks.canary, svcRouter, router)

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	err = router.SetRoutes(mocks.canary, 90, 10, false)
	require.NoError(t, err)

	primary, canary, _, err := router.GetRoutes(mocks.canary)

	assert.Equal(t, uint32(90), uint32(primary))
	assert.Equal(t, uint32(10), uint32(canary))
}

func initializeAndReconcileCanaryServices(t *testing.T,
	canary *flaggerv1.Canary,
	svcRouter *KubernetesDefaultRouter,
	router *GlooMeshRouter,
) {
	_, err := router.flaggerClient.GloomeshnetworkingV2().RouteTables("default").
		Create(context.TODO(), newTestRouteTable(), metav1.CreateOptions{})
	require.NoError(t, err)

	canary.Spec.RouteTableRef = testRouteTableRef
	err = svcRouter.Initialize(canary)
	require.NoError(t, err)
	err = svcRouter.Reconcile(canary)
	require.NoError(t, err)
}

func newTestRouteTable() *v2.RouteTable {
	v := &v2.RouteTable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
			Annotations: map[string]string{
				"foo": "bar",
			},
			Labels: map[string]string{
				"expose": "true",
			},
		},
		Spec: v2.RouteTableSpec{
			Hosts: []string{"podinfo.default.svc.cluster.local"},
			Http: []*v2.HTTPRoute{
				{
					Matchers: []*v2.HTTPRequestMatcher{},
					Delegate: &v2.DelegateAction{
						RouteTables: []*v2.ObjectSelector{
							{
								Name:      "podinfo-delegate",
								Namespace: "default",
							},
						},
					},
				},
			},
		},
	}
	return v
}
