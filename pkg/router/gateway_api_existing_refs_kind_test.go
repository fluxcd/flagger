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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayAPIRouter_ExistingRefs_Kind(t *testing.T) {
	t.Run("group spelled out", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		canary.Spec.Service.ExistingRefs[0].Group = "gateway.networking.k8s.io"
		router := newRefsRouter(t, canary, newIngressRouteFixture())

		require.NoError(t, router.Reconcile(canary))
		require.NoError(t, router.SetRoutes(canary, 70, 30, false))

		requireWeights(t, router, ingressRouteName, 70, 30)
	})

	t.Run("a kind other than HTTPRoute is rejected", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		canary.Spec.Service.ExistingRefs[0].Kind = "Ingress"
		router := newRefsRouter(t, canary, newIngressRouteFixture())

		err := router.Reconcile(canary)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `kind "Ingress" is not supported`)
		assert.Equal(t, newIngressRouteFixture().Spec, getTestRoute(t, router, ingressRouteName).Spec)
	})

	t.Run("the weights are not set on a kind other than HTTPRoute either", func(t *testing.T) {
		canary := newTestGatewayAPICanaryWithRefs(ingressRouteName)
		router := newRefsRouter(t, canary, newIngressRouteFixture())
		require.NoError(t, router.Reconcile(canary))

		// SetRoutes and GetRoutes do not validate the Canary: the kind is checked where the route is read
		canary.Spec.Service.ExistingRefs[0].Kind = "Ingress"
		err := router.SetRoutes(canary, 70, 30, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `kind "Ingress" is not supported`)
		_, _, _, err = router.GetRoutes(canary)
		require.Error(t, err)
		requireWeights(t, router, ingressRouteName, 100, 0)
	})
}
