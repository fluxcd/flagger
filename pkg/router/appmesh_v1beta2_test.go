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
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppmeshv1beta2Router_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshv1beta2Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	apexName, primaryName, canaryName := mocks.appmeshCanary.GetServiceNames()
	err := router.Reconcile(mocks.appmeshCanary)
	require.NoError(t, err)

	// check apex virtual service
	_, err = router.appmeshClient.AppmeshV1beta2().VirtualServices("default").Get(context.TODO(), apexName, metav1.GetOptions{})
	require.NoError(t, err)

	// check canary virtual service
	_, err = router.appmeshClient.AppmeshV1beta2().VirtualServices("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// check apex virtual router
	vrApex, err := router.appmeshClient.AppmeshV1beta2().VirtualRouters("default").Get(context.TODO(), apexName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vrApex.Spec.Routes[0].HTTPRoute.Action.WeightedTargets, 2)

	// check canary virtual router
	vrCanary, err := router.appmeshClient.AppmeshV1beta2().VirtualRouters("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// check if the canary virtual service routes all traffic to the canary virtual node
	target := vrCanary.Spec.Routes[0].HTTPRoute.Action.WeightedTargets[0]
	assert.Equal(t, canaryName, target.VirtualNodeRef.Name)
	assert.Equal(t, int64(100), target.Weight)

	// check primary virtual node
	vnPrimary, err := router.appmeshClient.AppmeshV1beta2().VirtualNodes("default").Get(context.TODO(), primaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// check FQDN
	primaryDNS := fmt.Sprintf("%s.%s.svc.cluster.local.", primaryName, mocks.appmeshCanary.Namespace)
	assert.Equal(t, primaryDNS, vnPrimary.Spec.ServiceDiscovery.DNS.Hostname)

	// check timeout
	assert.Equal(t, int64(30000), vrApex.Spec.Routes[0].HTTPRoute.Timeout.PerRequest.Value)
	assert.Equal(t, int64(30000), vnPrimary.Spec.Listeners[0].Timeout.HTTP.PerRequest.Value)

	// test backends update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), mocks.appmeshCanary.Name, metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	backends := cdClone.Spec.Service.Backends
	backends = append(backends, "test.example.com", "arn:aws:appmesh:eu-west-1:12345678910:mesh/my-mesh/virtualService/mytestservice")
	cdClone.Spec.Service.Backends = backends
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	// verify
	vnPrimary, err = router.appmeshClient.AppmeshV1beta2().VirtualNodes("default").Get(context.TODO(), primaryName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vnPrimary.Spec.Backends, 3)

	// update URI
	vrClone := vrApex.DeepCopy()
	vrClone.Spec.Routes[0].HTTPRoute.Match.Prefix = "api"
	vrApex, err = mocks.meshClient.AppmeshV1beta2().VirtualRouters("default").Update(context.TODO(), vrClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// undo URI change
	err = router.Reconcile(canary)
	require.NoError(t, err)
	vrApex, err = router.appmeshClient.AppmeshV1beta2().VirtualRouters("default").Get(context.TODO(), apexName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "/", vrApex.Spec.Routes[0].HTTPRoute.Match.Prefix)
}

func TestAppmeshv1beta2Router_GetSetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshv1beta2Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.appmeshCanary)
	require.NoError(t, err)

	err = router.SetRoutes(mocks.appmeshCanary, 60, 40, false)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.appmeshCanary)
	require.NoError(t, err)
	assert.Equal(t, 60, p)
	assert.Equal(t, 40, c)
	assert.False(t, m)
}

func TestAppmesv1beta2hRouter_ABTest(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshv1beta2Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	apexName, _, _ := mocks.abtest.GetServiceNames()
	err := router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	vrApex, err := router.appmeshClient.AppmeshV1beta2().VirtualRouters("default").Get(context.TODO(), apexName, metav1.GetOptions{})
	require.NoError(t, err)

	// check routes
	assert.Len(t, vrApex.Spec.Routes, 2)

	// check headers
	assert.GreaterOrEqual(t, len(vrApex.Spec.Routes[0].HTTPRoute.Match.Headers), 1, "Got no http match headers")
	assert.Equal(t, "x-user-type", vrApex.Spec.Routes[0].HTTPRoute.Match.Headers[0].Name)
	assert.Equal(t, "test", *vrApex.Spec.Routes[0].HTTPRoute.Match.Headers[0].Match.Exact)
}

func TestAppmeshv1beta2Router_Gateway(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshv1beta2Router{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	apexName, _, _ := mocks.appmeshCanary.GetServiceNames()
	err := router.Reconcile(mocks.appmeshCanary)
	require.NoError(t, err)

	vs, err := router.appmeshClient.AppmeshV1beta2().VirtualServices("default").Get(context.TODO(), apexName, metav1.GetOptions{})
	require.NoError(t, err)

	// check Flagger's gateway annotations
	assert.Equal(t, "true", vs.Annotations["gateway.appmesh.k8s.aws/expose"])
	assert.True(t, strings.Contains(vs.Annotations["gateway.appmesh.k8s.aws/domain"], mocks.appmeshCanary.Spec.Service.Hosts[0]))
	assert.Equal(t, mocks.appmeshCanary.Spec.Service.Timeout, vs.Annotations["gateway.appmesh.k8s.aws/timeout"])

	retries := vs.Annotations["gateway.appmesh.k8s.aws/retries"]
	assert.Equal(t, strconv.Itoa(mocks.appmeshCanary.Spec.Service.Retries.Attempts), retries)
}
