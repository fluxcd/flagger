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

func TestAppmeshRouter_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.appmeshCanary)
	require.NoError(t, err)

	primaryVirtualNodeName := fmt.Sprintf("%s-primary", mocks.appmeshCanary.Spec.TargetRef.Name)
	canaryVirtualNodeName := fmt.Sprintf("%s-canary", mocks.appmeshCanary.Spec.TargetRef.Name)

	// check virtual service
	vsName := fmt.Sprintf("%s.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	vs, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(context.TODO(), vsName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, mocks.appmeshCanary.Spec.Service.MeshName, vs.Spec.MeshName)
	assert.Len(t, vs.Spec.Routes[0].Http.Action.WeightedTargets, 2)

	// check virtual service routes all traffic to the primary virtual node
	assert.Equal(t, canaryVirtualNodeName, vs.Spec.Routes[0].Http.Action.WeightedTargets[0].VirtualNodeName)
	assert.Equal(t, int64(0), vs.Spec.Routes[0].Http.Action.WeightedTargets[0].Weight)
	assert.Equal(t, primaryVirtualNodeName, vs.Spec.Routes[0].Http.Action.WeightedTargets[1].VirtualNodeName)
	assert.Equal(t, int64(100), vs.Spec.Routes[0].Http.Action.WeightedTargets[1].Weight)

	// check canary virtual service
	vsCanaryName := fmt.Sprintf("%s-canary.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	vsCanary, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(context.TODO(), vsCanaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// check if the canary virtual service routes all traffic to the canary virtual node
	assert.Equal(t, canaryVirtualNodeName, vsCanary.Spec.Routes[0].Http.Action.WeightedTargets[0].VirtualNodeName)
	assert.Equal(t, int64(100), vsCanary.Spec.Routes[0].Http.Action.WeightedTargets[0].Weight)
	assert.Equal(t, primaryVirtualNodeName, vsCanary.Spec.Routes[0].Http.Action.WeightedTargets[1].VirtualNodeName)
	assert.Equal(t, int64(0), vsCanary.Spec.Routes[0].Http.Action.WeightedTargets[1].Weight)

	// check virtual node
	vnName := mocks.appmeshCanary.Spec.TargetRef.Name
	vn, err := router.appmeshClient.AppmeshV1beta1().VirtualNodes("default").Get(context.TODO(), vnName, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, vn.Spec.Logging)

	primaryDNS := fmt.Sprintf("%s-primary.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	assert.Equal(t, primaryDNS, vn.Spec.ServiceDiscovery.Dns.HostName)

	// test backends update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "appmesh", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Backends
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Backends = hosts
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	// verify
	vnCanaryName := fmt.Sprintf("%s-canary", mocks.appmeshCanary.Spec.TargetRef.Name)
	vnCanary, err := router.appmeshClient.AppmeshV1beta1().VirtualNodes("default").Get(context.TODO(), vnCanaryName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vnCanary.Spec.Backends, 2)

	// test weight update
	vsClone := vs.DeepCopy()
	vsClone.Spec.Routes[0].Http.Action.WeightedTargets[0].Weight = 50
	vsClone.Spec.Routes[0].Http.Action.WeightedTargets[1].Weight = 50
	vs, err = mocks.meshClient.AppmeshV1beta1().VirtualServices("default").Update(context.TODO(), vsClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	vs, err = router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(context.TODO(), vsName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(50), vs.Spec.Routes[0].Http.Action.WeightedTargets[0].Weight)

	// test URI update
	vsClone = vs.DeepCopy()
	vsClone.Spec.Routes[0].Http.Match.Prefix = "api"
	vs, err = mocks.meshClient.AppmeshV1beta1().VirtualServices("default").Update(context.TODO(), vsClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)
	vs, err = router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(context.TODO(), vsName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "/", vs.Spec.Routes[0].Http.Match.Prefix)
}

func TestAppmeshRouter_GetSetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshRouter{
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

func TestAppmeshRouter_ABTest(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	// check virtual service
	vsName := fmt.Sprintf("%s.%s", mocks.abtest.Spec.TargetRef.Name, mocks.abtest.Namespace)
	vs, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(context.TODO(), vsName, metav1.GetOptions{})
	require.NoError(t, err)

	// check virtual service
	assert.Len(t, vs.Spec.Routes, 2)

	// check headers
	assert.GreaterOrEqual(t, len(vs.Spec.Routes[0].Http.Match.Headers), 1, "Got no http match headers")
	assert.Equal(t, "x-user-type", vs.Spec.Routes[0].Http.Match.Headers[0].Name)
	assert.Equal(t, "test", *vs.Spec.Routes[0].Http.Match.Headers[0].Match.Exact)
}

func TestAppmeshRouter_Gateway(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.appmeshCanary)
	require.NoError(t, err)

	// check virtual service
	vsName := fmt.Sprintf("%s.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	vs, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(context.TODO(), vsName, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "true", vs.Annotations["gateway.appmesh.k8s.aws/expose"])
	assert.True(t, strings.Contains(vs.Annotations["gateway.appmesh.k8s.aws/domain"], mocks.appmeshCanary.Spec.Service.Hosts[0]))
	assert.Equal(t, mocks.appmeshCanary.Spec.Service.Timeout, vs.Annotations["gateway.appmesh.k8s.aws/timeout"])

	retries := vs.Annotations["gateway.appmesh.k8s.aws/retries"]
	assert.Equal(t, strconv.Itoa(mocks.appmeshCanary.Spec.Service.Retries.Attempts), retries)
}

func TestAppmeshRouter_ProgressiveInit(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	canary := mocks.appmeshCanary
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

func TestAppmeshRouter_ProgressiveUpdate(t *testing.T) {
	mocks := newFixture(nil)
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	canary := mocks.appmeshCanary
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
