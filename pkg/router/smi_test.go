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
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	smiv1 "github.com/fluxcd/flagger/pkg/apis/smi/v1alpha1"
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
	require.NoError(t, err)

	// test insert
	ts, err := router.smiClient.SplitV1alpha1().TrafficSplits("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
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

	assert.Equal(t, strconv.Itoa(100), pRoute.Weight.String())
	assert.Equal(t, strconv.Itoa(0), cRoute.Weight.String())

	// test update
	host := "test"
	canary.Spec.Service.Name = host

	err = router.Reconcile(canary)
	require.NoError(t, err)

	ts, err = router.smiClient.SplitV1alpha1().TrafficSplits("default").Get(context.TODO(), "test", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, host, ts.Spec.Service)
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
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	p = 50
	c = 50
	m = false

	err = router.SetRoutes(mocks.canary, p, c, m)
	require.NoError(t, err)

	ts, err := router.smiClient.SplitV1alpha1().TrafficSplits("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
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

	assert.Equal(t, strconv.Itoa(p), pRoute.Weight.String())
	assert.Equal(t, strconv.Itoa(c), cRoute.Weight.String())
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
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.False(t, m)
}

func TestSmiRouter_ProgressiveInit(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &SmiRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	canarySpec := &canary.Spec
	canarySpec.ProgressiveInitialization = true
	canarySpec.Analysis.StepWeightPromotion = canarySpec.Analysis.StepWeight
	err := router.Reconcile(canary)
	require.NoError(t, err)

	// check virtual service routes all traffic to canary initially
	primaryWeight, canaryWeight, _, err := router.GetRoutes(canary)
	assert.Equal(t, 0, primaryWeight)
	assert.Equal(t, 100, canaryWeight)
}

func TestSmiRouter_ProgressiveUpdate(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &SmiRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		smiClient:     mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	canary.Spec.Analysis.StepWeightPromotion = canary.Spec.Analysis.StepWeight
	err := router.Reconcile(canary)
	require.NoError(t, err)

	// check virtual service routes all traffic to primary initially
	primaryWeight, canaryWeight, _, err := router.GetRoutes(canary)
	assert.Equal(t, 100, primaryWeight)
	assert.Equal(t, 0, canaryWeight)

	// test progressive update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	cdClone := cd.DeepCopy()
	cdClone.Spec.ProgressiveInitialization = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply
	err = router.Reconcile(canary)
	require.NoError(t, err)

	// verify virtual service traffic remains intact
	primaryWeight, canaryWeight, _, err = router.GetRoutes(canary)
	assert.Equal(t, 100, primaryWeight)
	assert.Equal(t, 0, canaryWeight)
}
