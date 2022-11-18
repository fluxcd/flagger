/*
Copyright 2022 The Flux authors

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
	"fmt"
	"testing"

	"github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApisixRouter_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	mocks.canary.Spec.RouteRef = &v1beta1.LocalObjectReference{
		Name:       "podinfo",
		Kind:       "ApisixRoute",
		APIVersion: "apisix.apache.org/v2",
	}
	router := &ApisixRouter{
		apisixClient: mocks.flaggerClient,
		logger:       mocks.logger,
	}
	apexName, _, _ := mocks.canary.GetServiceNames()
	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)
	canaryName := fmt.Sprintf("%s-%s-canary", mocks.canary.Spec.RouteRef.Name, apexName)
	arCanary, err := router.apisixClient.ApisixV2().ApisixRoutes("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(arCanary.Spec.HTTP[0].Backends))
}

func TestApisixRouter_GetSetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	mocks.canary.Spec.RouteRef = &v1beta1.LocalObjectReference{
		Name:       "podinfo",
		Kind:       "ApisixRoute",
		APIVersion: "apisix.apache.org/v2",
	}
	router := &ApisixRouter{
		apisixClient: mocks.flaggerClient,
		logger:       mocks.logger,
	}
	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)
	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.False(t, m)

	p = 50
	c = 50
	m = false
	err = router.SetRoutes(mocks.canary, p, c, m)
	require.NoError(t, err)

	p, c, m, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 50, p)
	assert.Equal(t, 50, c)
	assert.False(t, m)

	apexName, _, _ := mocks.canary.GetServiceNames()
	canaryName := fmt.Sprintf("%s-%s-canary", mocks.canary.Spec.RouteRef.Name, apexName)
	arRouter, err := router.apisixClient.ApisixV2().ApisixRoutes("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(arRouter.Spec.HTTP[0].Backends))
	assert.Equal(t, 50, *arRouter.Spec.HTTP[0].Backends[0].Weight)
	assert.Equal(t, 50, *arRouter.Spec.HTTP[0].Backends[1].Weight)
}
