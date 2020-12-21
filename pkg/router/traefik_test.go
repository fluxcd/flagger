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

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTraefikRouter_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	mocks.canary.Spec.Service.Apex = &flaggerv1.CustomMetadata{
		Labels: map[string]string{
			"test": "label",
		},
		Annotations: map[string]string{
			"test": "annotation",
		},
	}

	router := &TraefikRouter{
		traefikClient: mocks.meshClient,
		logger:        mocks.logger,
	}

	assert.NoError(t, router.Reconcile(mocks.canary))
	ts, err := router.traefikClient.TraefikV1alpha1().TraefikServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	assert.NoError(t, err)

	services := ts.Spec.Weighted.Services
	assert.Len(t, services, 1)
	assert.Equal(t, uint(100), services[0].Weight)

	assert.Equal(t, ts.ObjectMeta.Labels, mocks.canary.Spec.Service.Apex.Labels)
	assert.Equal(t, ts.ObjectMeta.Annotations, mocks.canary.Spec.Service.Apex.Annotations)

	for _, tt := range []struct {
		name        string
		primary     int
		canary      int
		servicesLen int
	}{
		{
			name:        "should not change weights when canary is progressing",
			primary:     60,
			canary:      40,
			servicesLen: 2,
		},
		{
			name:        "should not change weights when canary isn't progressing",
			primary:     100,
			canary:      0,
			servicesLen: 1,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, router.Reconcile(mocks.canary))
			assert.NoError(t, router.SetRoutes(mocks.canary, tt.primary, tt.canary, false))
			assert.NoError(t, router.Reconcile(mocks.canary))

			ts, err := router.traefikClient.TraefikV1alpha1().TraefikServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
			assert.NoError(t, err)

			services := ts.Spec.Weighted.Services
			assert.Len(t, services, tt.servicesLen)
			assert.Equal(t, uint(tt.primary), services[0].Weight)
			if tt.canary > 0 {
				assert.Equal(t, uint(tt.canary), services[1].Weight)
			}
		})

	}
}

func TestTraefikRouter_SetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &TraefikRouter{
		traefikClient: mocks.meshClient,
		logger:        mocks.logger,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	_, _, _, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	for _, tt := range []struct {
		name        string
		primary     int
		canary      int
		servicesLen int
	}{
		{name: "0%", primary: 100, canary: 0, servicesLen: 1},
		{name: "20%", primary: 80, canary: 20, servicesLen: 2},
		{name: "40%", primary: 60, canary: 40, servicesLen: 2},
		{name: "60%", primary: 40, canary: 60, servicesLen: 2},
		{name: "80%", primary: 20, canary: 80, servicesLen: 2},
		{name: "100%", primary: 0, canary: 100, servicesLen: 2},
		{name: "0% (promote)", primary: 100, canary: 0, servicesLen: 1},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err = router.SetRoutes(mocks.canary, tt.primary, tt.canary, false)
			require.NoError(t, err)

			ts, err := router.traefikClient.TraefikV1alpha1().TraefikServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
			assert.NoError(t, err)

			services := ts.Spec.Weighted.Services
			assert.Len(t, services, tt.servicesLen)
			assert.Equal(t, uint(tt.primary), services[0].Weight)
			if tt.canary > 0 {
				assert.Equal(t, uint(tt.canary), services[1].Weight)
			}

		})
	}
}

func TestTraefikRouter_GetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &TraefikRouter{
		traefikClient: mocks.meshClient,
		logger:        mocks.logger,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.False(t, m)
}
