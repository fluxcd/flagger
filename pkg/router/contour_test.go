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

func TestContourRouter_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	router := &ContourRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		contourClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
		ingressClass:  "contour",
	}

	// init
	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// test insert
	proxy, err := router.contourClient.ProjectcontourV1().HTTPProxies("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	services := proxy.Spec.Routes[0].Services
	require.Len(t, services, 2)
	assert.Equal(t, uint32(100), services[0].Weight)
	assert.Equal(t, uint32(0), services[1].Weight)
	assert.Equal(t, "contour", proxy.Annotations["projectcontour.io/ingress.class"])

	// test update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	cdClone.Spec.Service.Port = 8080
	cdClone.Spec.Service.Timeout = "1m"
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 8080, proxy.Spec.Routes[0].Services[0].Port)
	assert.Equal(t, "1m", proxy.Spec.Routes[0].TimeoutPolicy.Response)
	assert.Equal(t, "/podinfo", proxy.Spec.Routes[0].Conditions[0].Prefix)
	assert.Equal(t, uint32(10), proxy.Spec.Routes[0].RetryPolicy.NumRetries)

	// test headers update
	cd, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone = cd.DeepCopy()
	cdClone.Spec.Analysis.Iterations = 5
	cdClone.Spec.Analysis.Match = newTestABTest().Spec.Analysis.Match
	canary, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test", proxy.Spec.Routes[0].Conditions[0].Header.Exact)
}

func TestContourRouter_Routes(t *testing.T) {
	mocks := newFixture(nil)
	router := &ContourRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		contourClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// init
	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// test set routers
	err = router.SetRoutes(mocks.canary, 50, 50, false)
	require.NoError(t, err)

	proxy, err := router.contourClient.ProjectcontourV1().HTTPProxies("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	primary := proxy.Spec.Routes[0].Services[0]
	assert.Equal(t, uint32(50), primary.Weight)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// test get routers
	_, cw, _, err := router.GetRoutes(cd)
	require.NoError(t, err)
	assert.Equal(t, 50, cw)

	// test update to A/B
	cdClone := cd.DeepCopy()
	cdClone.Spec.Analysis.Iterations = 5
	cdClone.Spec.Analysis.Match = newTestABTest().Spec.Analysis.Match
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = router.Reconcile(canary)
	require.NoError(t, err)

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	primary = proxy.Spec.Routes[0].Services[0]
	assert.Equal(t, uint32(100), primary.Weight)

	primary = proxy.Spec.Routes[1].Services[0]
	assert.Equal(t, uint32(100), primary.Weight)

	// test set routers for A/B
	err = router.SetRoutes(canary, 0, 100, false)
	require.NoError(t, err)

	proxy, err = router.contourClient.ProjectcontourV1().HTTPProxies("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	primary = proxy.Spec.Routes[0].Services[0]
	assert.Equal(t, uint32(0), primary.Weight)

	primary = proxy.Spec.Routes[1].Services[0]
	assert.Equal(t, uint32(100), primary.Weight)
}
