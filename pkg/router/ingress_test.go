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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/fluxcd/flagger/pkg/apis/istio/v1alpha3"
)

func TestIngressRouter_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	router := &IngressRouter{
		logger:            mocks.logger,
		kubeClient:        mocks.kubeClient,
		annotationsPrefix: "custom.ingress.kubernetes.io",
	}

	err := router.Reconcile(mocks.ingressCanary)
	require.NoError(t, err)

	canaryAn := "custom.ingress.kubernetes.io/canary"
	canaryWeightAn := "custom.ingress.kubernetes.io/canary-weight"

	canaryName := fmt.Sprintf("%s-canary", mocks.ingressCanary.Spec.IngressRef.Name)
	inCanary, err := router.kubeClient.NetworkingV1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	annotation := inCanary.Annotations["kustomize.toolkit.fluxcd.io/checksum"]
	assert.Equal(t, "", annotation)

	// test initialisation
	assert.Equal(t, "true", inCanary.Annotations[canaryAn])
	assert.Equal(t, "0", inCanary.Annotations[canaryWeightAn])
}

func TestIngressRouter_GetSetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &IngressRouter{
		logger:            mocks.logger,
		kubeClient:        mocks.kubeClient,
		annotationsPrefix: "prefix1.nginx.ingress.kubernetes.io",
	}

	err := router.Reconcile(mocks.ingressCanary)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.ingressCanary)
	require.NoError(t, err)

	p = 50
	c = 50
	m = false

	err = router.SetRoutes(mocks.ingressCanary, p, c, m)
	require.NoError(t, err)

	canaryAn := "prefix1.nginx.ingress.kubernetes.io/canary"
	canaryWeightAn := "prefix1.nginx.ingress.kubernetes.io/canary-weight"

	canaryName := fmt.Sprintf("%s-canary", mocks.ingressCanary.Spec.IngressRef.Name)
	inCanary, err := router.kubeClient.NetworkingV1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// test rollout
	assert.Equal(t, "true", inCanary.Annotations[canaryAn])
	assert.Equal(t, "50", inCanary.Annotations[canaryWeightAn])

	p = 100
	c = 0
	m = false

	err = router.SetRoutes(mocks.ingressCanary, p, c, m)
	require.NoError(t, err)

	inCanary, err = router.kubeClient.NetworkingV1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// test promotion
	assert.Equal(t, "true", inCanary.Annotations[canaryAn])
	assert.Equal(t, "0", inCanary.Annotations[canaryWeightAn])
}

func TestIngressRouter_ABTest(t *testing.T) {
	mocks := newFixture(nil)
	router := &IngressRouter{
		logger:            mocks.logger,
		kubeClient:        mocks.kubeClient,
		annotationsPrefix: "nginx.ingress.kubernetes.io",
	}

	tables := []struct {
		makeCanary func() *flaggerv1.Canary
		annotation string
	}{
		// Header exact match
		{
			makeCanary: func() *flaggerv1.Canary {
				mocks.ingressCanary.Spec.Analysis.Iterations = 1
				mocks.ingressCanary.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"x-user-type": {
								Exact: "test",
							},
						},
					},
				}
				return mocks.ingressCanary
			},
			annotation: router.GetAnnotationWithPrefix("canary-by-header-value"),
		},
		// Header regex match
		{
			makeCanary: func() *flaggerv1.Canary {
				mocks.ingressCanary.Spec.Analysis.Iterations = 1
				mocks.ingressCanary.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"x-user-type": {
								Regex: "test",
							},
						},
					},
				}
				return mocks.ingressCanary
			},
			annotation: router.GetAnnotationWithPrefix("canary-by-header-pattern"),
		},
		// Cookie exact match
		{
			makeCanary: func() *flaggerv1.Canary {
				mocks.ingressCanary.Spec.Analysis.Iterations = 1
				mocks.ingressCanary.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"cookie": {
								Exact: "test",
							},
						},
					},
				}
				return mocks.ingressCanary
			},
			annotation: router.GetAnnotationWithPrefix("canary-by-cookie"),
		},
	}

	for _, table := range tables {
		err := router.Reconcile(table.makeCanary())
		require.NoError(t, err)

		err = router.SetRoutes(table.makeCanary(), 50, 50, false)
		require.NoError(t, err)

		canaryAn := router.GetAnnotationWithPrefix("canary")

		canaryName := fmt.Sprintf("%s-canary", table.makeCanary().Spec.IngressRef.Name)
		inCanary, err := router.kubeClient.NetworkingV1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
		require.NoError(t, err)

		// test initialisation
		assert.Equal(t, "true", inCanary.Annotations[canaryAn])
		assert.Equal(t, "test", inCanary.Annotations[table.annotation])
		assert.Equal(t, "", inCanary.Annotations["kustomize.toolkit.fluxcd.io/checksum"])
	}
}
