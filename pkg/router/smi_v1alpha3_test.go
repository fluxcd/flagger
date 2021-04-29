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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	smiv1 "github.com/fluxcd/flagger/pkg/apis/smi/v1alpha3"
)

func TestSmiv1alpha3Router_Sync(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &Smiv1alpha3Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	// test insert
	ts, err := router.smiClient.SplitV1alpha3().TrafficSplits("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	dests := ts.Spec.Backends
	assert.Len(t, dests, 2)

	apexName, primaryName, canaryName := canary.GetServiceNames()
	assert.Equal(t, ts.Spec.Service, apexName)

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

	assert.Equal(t, 100, pRoute.Weight)
	assert.Equal(t, 0, cRoute.Weight)

	// test update
	host := "test"
	canary.Spec.Service.Name = host

	err = router.Reconcile(canary)
	require.NoError(t, err)

	ts, err = router.smiClient.SplitV1alpha3().TrafficSplits("default").Get(context.TODO(), "test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, host, ts.Spec.Service)
}

func TestSmiv1alpha3Router_SetRoutes(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &Smiv1alpha3Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	p = 50
	c = 50
	m = false

	err = router.SetRoutes(mocks.canary, p, c, m)
	require.NoError(t, err)

	ts, err := router.smiClient.SplitV1alpha3().TrafficSplits("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

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

	assert.Equal(t, p, pRoute.Weight)
	assert.Equal(t, c, cRoute.Weight)
}

func TestSmiv1alpha3Router_GetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &Smiv1alpha3Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
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
