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
)

func TestKumaRouter_Reconcile(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &KumaRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kumaClient:    mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// init
	err := router.Reconcile(canary)
	require.NoError(t, err)

	// test insert
	trafficRoute, err := router.kumaClient.KumaV1alpha1().TrafficRoutes().Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	splits := trafficRoute.Spec.Conf.Split
	require.Len(t, splits, 2)
	assert.Equal(t, uint32(100), splits[0].Weight)
	assert.Equal(t, uint32(0), splits[1].Weight)

}

func TestKumaRouter_Routes(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &KumaRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kumaClient:    mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// init
	err := router.Reconcile(canary)
	require.NoError(t, err)

	// test set routers
	err = router.SetRoutes(canary, 50, 50, false)
	require.NoError(t, err)

	trafficRoute, err := router.kumaClient.KumaV1alpha1().TrafficRoutes().Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	primary := trafficRoute.Spec.Conf.Split[0]
	assert.Equal(t, uint32(50), primary.Weight)

}

func TestKumaRouter_ProgressiveInit(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &KumaRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kumaClient:    mocks.meshClient,
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

func TestKumaRouter_ProgressiveUpdate(t *testing.T) {
	canary := newTestSMICanary()
	mocks := newFixture(canary)
	router := &KumaRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		kumaClient:    mocks.meshClient,
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
