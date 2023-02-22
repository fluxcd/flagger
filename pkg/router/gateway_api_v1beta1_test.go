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

func TestGatewayAPIV1Beta1Router_Reconcile(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIV1Beta1Router{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	httpRoute, err := router.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	routeRules := httpRoute.Spec.Rules
	require.Equal(t, len(routeRules), 1)

	backendRefs := routeRules[0].BackendRefs
	require.Equal(t, len(backendRefs), 2)
	assert.Equal(t, int32(100), *backendRefs[0].Weight)
	assert.Equal(t, int32(0), *backendRefs[1].Weight)
}

func TestGatewayAPIV1Beta1Router_Routes(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIV1Beta1Router{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	err = router.SetRoutes(canary, 50, 50, false)
	require.NoError(t, err)

	httpRoute, err := router.gatewayAPIClient.GatewayapiV1beta1().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	primary := httpRoute.Spec.Rules[0].BackendRefs[0]
	assert.Equal(t, int32(50), *primary.Weight)
}

func TestGatewayAPIV1Beta1Router_ProgressiveInit(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIV1Beta1Router{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
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

func TestGatewayAPIV1Beta1Router_ProgressiveUpdate(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIV1Beta1Router{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
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
