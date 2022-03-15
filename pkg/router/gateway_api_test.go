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

func TestGatewayAPIRouter_Reconcile(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	httpRoute, err := router.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	routeRules := httpRoute.Spec.Rules
	require.Equal(t, len(routeRules), 1)

	backendRefs := routeRules[0].BackendRefs
	require.Equal(t, len(backendRefs), 2)
	assert.Equal(t, int32(100), *backendRefs[0].Weight)
	assert.Equal(t, int32(0), *backendRefs[1].Weight)
}

func TestGatewayAPIRouter_Routes(t *testing.T) {
	canary := newTestGatewayAPICanary()
	mocks := newFixture(canary)
	router := &GatewayAPIRouter{
		gatewayAPIClient: mocks.meshClient,
		kubeClient:       mocks.kubeClient,
		logger:           mocks.logger,
	}

	err := router.Reconcile(canary)
	require.NoError(t, err)

	err = router.SetRoutes(canary, 50, 50, false)
	require.NoError(t, err)

	httpRoute, err := router.gatewayAPIClient.GatewayapiV1alpha2().HTTPRoutes("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	primary := httpRoute.Spec.Rules[0].BackendRefs[0]
	assert.Equal(t, int32(50), *primary.Weight)
}
