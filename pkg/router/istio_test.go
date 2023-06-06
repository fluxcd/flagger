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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/fluxcd/flagger/pkg/apis/istio/v1alpha3"
)

func TestIstioRouter_Sync(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// test insert
	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	require.Len(t, vs.Spec.Http[0].Route, 2)

	// test update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Hosts
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Hosts = hosts
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Hosts, 2)

	// test drift
	vsClone := vs.DeepCopy()
	gateways := vsClone.Spec.Gateways
	gateways = append(gateways, "test-gateway.istio-system")
	vsClone.Spec.Gateways = gateways
	totalGateways := len(mocks.canary.Spec.Service.Gateways)

	vsGateways, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Update(context.TODO(), vsClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	totalGateways++
	assert.Len(t, vsGateways.Spec.Gateways, totalGateways)

	// undo change
	totalGateways--
	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Gateways, totalGateways)
}

func TestIstioRouter_SetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	pHost := fmt.Sprintf("%s-primary", mocks.canary.Spec.TargetRef.Name)
	cHost := fmt.Sprintf("%s-canary", mocks.canary.Spec.TargetRef.Name)

	t.Run("normal", func(t *testing.T) {
		p, c := 60, 40
		err := router.SetRoutes(mocks.canary, p, c, false)
		require.NoError(t, err)

		vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		var pRoute, cRoute istiov1alpha3.HTTPRouteDestination
		var mirror *istiov1alpha3.Destination
		for _, http := range vs.Spec.Http {
			for _, route := range http.Route {
				if route.Destination.Host == pHost {
					pRoute = route
				}
				if route.Destination.Host == cHost {
					cRoute = route
					mirror = http.Mirror
				}
			}
		}

		assert.Equal(t, p, pRoute.Weight)
		assert.Equal(t, c, cRoute.Weight)
		assert.Nil(t, mirror)

	})

	t.Run("session affinity", func(t *testing.T) {
		canary := mocks.canary.DeepCopy()
		cookieKey := "flagger-cookie"
		// enable session affinity and start canary run
		canary.Spec.Analysis.SessionAffinity = &v1beta1.SessionAffinity{
			CookieName: cookieKey,
			MaxAge:     300,
		}
		err := router.SetRoutes(canary, 0, 10, false)

		vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Len(t, vs.Spec.Http, 2)
		stickyRoute := vs.Spec.Http[0]
		weightedRoute := vs.Spec.Http[1]

		// stickyRoute should match against a cookie and direct all traffic to the canary when a canary run is active.
		var found bool
		for _, match := range stickyRoute.Match {
			if val, ok := match.Headers[cookieHeader]; ok {
				found = true
				assert.True(t, strings.Contains(val.Regex, cookieKey))
				for _, routeDest := range stickyRoute.Route {
					if routeDest.Destination.Host == pHost {
						assert.Equal(t, 0, routeDest.Weight)
					}
					if routeDest.Destination.Host == cHost {
						assert.Equal(t, 100, routeDest.Weight)
					}
				}
			}
		}
		assert.True(t, found)

		// weightedRoute should do regular weight based routing and inject the Set-Cookie header
		// for all responses returned from the canary deployment.
		for _, routeDest := range weightedRoute.Route {
			if routeDest.Destination.Host == pHost {
				assert.Equal(t, 0, routeDest.Weight)
			}
			if routeDest.Destination.Host == cHost {
				assert.Equal(t, 10, routeDest.Weight)
				val, ok := routeDest.Headers.Response.Add[setCookieHeader]
				assert.True(t, ok)
				assert.True(t, strings.HasPrefix(val, cookieKey))
				assert.True(t, strings.Contains(val, "Max-Age=300"))
			}
		}
		assert.True(t, strings.HasPrefix(canary.Status.SessionAffinityCookie, cookieKey))

		// reconcile canary, destination rules, virtual services
		err = router.Reconcile(canary)
		require.NoError(t, err)

		reconciledVS, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		// routes should not be changed.
		assert.Len(t, vs.Spec.Http, 2)
		assert.NotNil(t, reconciledVS)
		assert.Equal(t, cmp.Diff(reconciledVS.Spec.Http[0], stickyRoute), "")
		assert.Equal(t, cmp.Diff(reconciledVS.Spec.Http[1], weightedRoute), "")

		// further continue the canary run
		err = router.SetRoutes(canary, 50, 50, false)
		require.NoError(t, err)

		vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Len(t, vs.Spec.Http, 2)
		stickyRoute = vs.Spec.Http[0]
		weightedRoute = vs.Spec.Http[1]

		found = false
		for _, match := range stickyRoute.Match {
			if val, ok := match.Headers[cookieHeader]; ok {
				found = true
				assert.True(t, strings.Contains(val.Regex, cookieKey))
				for _, routeDest := range stickyRoute.Route {
					if routeDest.Destination.Host == pHost {
						assert.Equal(t, 0, routeDest.Weight)
					}
					if routeDest.Destination.Host == cHost {
						assert.Equal(t, 100, routeDest.Weight)
					}
				}
			}
		}
		assert.True(t, found)

		for _, routeDest := range weightedRoute.Route {
			if routeDest.Destination.Host == pHost {
				assert.Equal(t, 50, routeDest.Weight)
			}
			if routeDest.Destination.Host == cHost {
				assert.Equal(t, 50, routeDest.Weight)
				val, ok := routeDest.Headers.Response.Add[setCookieHeader]
				assert.True(t, ok)
				assert.True(t, strings.HasPrefix(val, cookieKey))
				assert.True(t, strings.Contains(val, "Max-Age=300"))
			}
		}
		assert.True(t, strings.HasPrefix(canary.Status.SessionAffinityCookie, cookieKey))
		sessionAffinityCookie := canary.Status.SessionAffinityCookie

		// promotion
		err = router.SetRoutes(canary, 100, 0, false)
		require.NoError(t, err)

		vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Len(t, vs.Spec.Http, 2)
		stickyRoute = vs.Spec.Http[0]
		weightedRoute = vs.Spec.Http[1]

		found = false
		for _, match := range stickyRoute.Match {
			if val, ok := match.Headers[cookieHeader]; ok {
				found = true
				assert.True(t, strings.Contains(val.Regex, cookieKey))
				for _, routeDest := range stickyRoute.Route {
					if routeDest.Destination.Host == pHost {
						assert.Equal(t, 100, routeDest.Weight)
					}
					if routeDest.Destination.Host == cHost {
						assert.Equal(t, 0, routeDest.Weight)
					}
				}
			}
		}
		assert.True(t, found)

		assert.Equal(t, canary.Status.SessionAffinityCookie, "")
		assert.Equal(t, canary.Status.PreviousSessionAffinityCookie, sessionAffinityCookie)

		val, ok := stickyRoute.Headers.Response.Add[setCookieHeader]
		assert.True(t, ok)
		assert.True(t, strings.HasPrefix(val, sessionAffinityCookie))
		assert.True(t, strings.Contains(val, "Max-Age=-1"))

		// delete the Set-Cookie header from responses returned by the weighted route
		for _, routeDest := range weightedRoute.Route {
			if routeDest.Destination.Host == pHost {
				assert.Equal(t, 100, routeDest.Weight)
			}
			if routeDest.Destination.Host == cHost {
				assert.Equal(t, 0, routeDest.Weight)
				if routeDest.Headers != nil && routeDest.Headers.Response != nil {
					_, ok := routeDest.Headers.Response.Add[setCookieHeader]
					assert.False(t, ok)
				}
			}
		}
	})

	t.Run("mirror", func(t *testing.T) {
		for _, w := range []int{0, 10, 50} {
			p, c := 100, 0

			// set mirror weight
			mocks.canary.Spec.Analysis.MirrorWeight = w
			err := router.SetRoutes(mocks.canary, p, c, true)
			require.NoError(t, err)

			vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
			require.NoError(t, err)

			var pRoute, cRoute istiov1alpha3.HTTPRouteDestination
			var mirror *istiov1alpha3.Destination
			var mirrorWeight *istiov1alpha3.Percent
			for _, http := range vs.Spec.Http {
				for _, route := range http.Route {
					if route.Destination.Host == pHost {
						pRoute = route
					}
					if route.Destination.Host == cHost {
						cRoute = route
						mirror = http.Mirror
						mirrorWeight = http.MirrorPercentage
					}
				}
			}

			assert.Equal(t, p, pRoute.Weight)
			assert.Equal(t, c, cRoute.Weight)
			if assert.NotNil(t, mirror) {
				assert.Equal(t, cHost, mirror.Host)
			}

			if w > 0 && assert.NotNil(t, mirrorWeight) {
				assert.Equal(t, w, int(mirrorWeight.Value))
			} else {
				assert.Nil(t, mirrorWeight)
			}
		}
	})
}

func TestIstioRouter_GetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.False(t, m)

	mocks.canary = newTestMirror()

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	p, c, m, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)

	// A Canary resource with mirror on does not automatically create mirroring
	// in the virtual server (mirroring is activated as a temporary stage).
	assert.False(t, m)

	// Adjust vs to activate mirroring.
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cHost := fmt.Sprintf("%s-canary", mocks.canary.Spec.TargetRef.Name)
	for i, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == cHost {
				vs.Spec.Http[i].Mirror = &istiov1alpha3.Destination{
					Host: cHost,
				}
			}
		}
	}
	_, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices(mocks.canary.Namespace).Update(context.TODO(), vs, metav1.UpdateOptions{})
	require.NoError(t, err)

	p, c, m, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.True(t, m)
}

func TestIstioRouter_HTTPRequestHeaders(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	assert.Equal(t, "15000", vs.Spec.Http[0].Headers.Request.Add["x-envoy-upstream-rq-timeout-ms"])
	assert.Equal(t, "test", vs.Spec.Http[0].Headers.Request.Remove[0])
	assert.Equal(t, "token", vs.Spec.Http[0].Headers.Response.Remove[0])
}

func TestIstioRouter_CORS(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	assert.NotNil(t, vs.Spec.Http[0].CorsPolicy)
	assert.Len(t, vs.Spec.Http[0].CorsPolicy.AllowMethods, 2)
}

func TestIstioRouter_ABTest(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	// test insert
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)

	p := 0
	c := 100
	m := false

	err = router.SetRoutes(mocks.abtest, p, c, m)
	require.NoError(t, err)

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)

	pHost := fmt.Sprintf("%s-primary", mocks.abtest.Spec.TargetRef.Name)
	cHost := fmt.Sprintf("%s-canary", mocks.abtest.Spec.TargetRef.Name)
	pRoute := istiov1alpha3.HTTPRouteDestination{}
	cRoute := istiov1alpha3.HTTPRouteDestination{}
	var mirror *istiov1alpha3.Destination

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == pHost {
				pRoute = route
			}
			if route.Destination.Host == cHost {
				cRoute = route
				mirror = http.Mirror
			}
		}
	}

	assert.Equal(t, p, pRoute.Weight)
	assert.Equal(t, c, cRoute.Weight)
	assert.Nil(t, mirror)
}

func TestIstioRouter_GatewayPort(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	port := vs.Spec.Http[0].Route[0].Destination.Port.Number
	assert.Equal(t, uint32(mocks.canary.Spec.Service.Port), port)
}

func TestIstioRouter_Delegate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		mocks := newFixture(nil)
		mocks.canary.Spec.Service.Hosts = []string{}
		mocks.canary.Spec.Service.Gateways = []string{}
		mocks.canary.Spec.Service.Delegation = true

		router := &IstioRouter{
			logger:        mocks.logger,
			flaggerClient: mocks.flaggerClient,
			istioClient:   mocks.meshClient,
			kubeClient:    mocks.kubeClient,
		}

		err := router.Reconcile(mocks.canary)
		require.NoError(t, err)

		vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Equal(t, 0, len(vs.Spec.Hosts))
		assert.Equal(t, 0, len(vs.Spec.Gateways))

		port := vs.Spec.Http[0].Route[0].Destination.Port.Number
		assert.Equal(t, uint32(mocks.canary.Spec.Service.Port), port)
	})

	t.Run("invalid", func(t *testing.T) {
		mocks := newFixture(nil)
		if len(mocks.canary.Spec.Service.Gateways) == 0 {
			// in this case, the gateways or hosts should not be not empty because it requires to cause an error.
			mocks.canary.Spec.Service.Gateways = []string{
				"public-gateway.istio",
				"mesh",
			}
		}
		mocks.canary.Spec.Service.Delegation = true

		router := &IstioRouter{
			logger:        mocks.logger,
			flaggerClient: mocks.flaggerClient,
			istioClient:   mocks.meshClient,
			kubeClient:    mocks.kubeClient,
		}

		err := router.Reconcile(mocks.canary)
		require.Error(t, err)
	})
}

func TestIstioRouter_Finalize(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	flaggerSpec := &istiov1alpha3.VirtualServiceSpec{
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match:      mocks.canary.Spec.Service.Match,
				Rewrite:    mocks.canary.Spec.Service.Rewrite,
				Timeout:    mocks.canary.Spec.Service.Timeout,
				Retries:    mocks.canary.Spec.Service.Retries,
				CorsPolicy: mocks.canary.Spec.Service.CorsPolicy,
			},
		},
	}

	kubectlSpec := &istiov1alpha3.VirtualServiceSpec{
		Hosts:    []string{"podinfo"},
		Gateways: []string{"ingressgateway.istio-system.svc.cluster.local"},
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match: nil,
				Route: []istiov1alpha3.HTTPRouteDestination{
					{
						Destination: istiov1alpha3.Destination{Host: "podinfo"},
					},
				},
			},
		},
	}

	tables := []struct {
		router        *IstioRouter
		spec          *istiov1alpha3.VirtualServiceSpec
		shouldError   bool
		createVS      bool
		canary        *v1beta1.Canary
		callReconcile bool
		annotation    string
	}{
		// VS not found
		{router: router, spec: nil, shouldError: true, createVS: false, canary: mocks.canary, callReconcile: false, annotation: ""},
		// No annotation found but still finalizes
		{router: router, spec: nil, shouldError: false, createVS: false, canary: mocks.canary, callReconcile: true, annotation: ""},
		// Spec should match annotation after finalize
		{router: router, spec: flaggerSpec, shouldError: false, createVS: true, canary: mocks.canary, callReconcile: true, annotation: "flagger"},
		// Need to test kubectl annotation
		{router: router, spec: kubectlSpec, shouldError: false, createVS: true, canary: mocks.canary, callReconcile: true, annotation: "kubectl"},
	}

	for _, table := range tables {
		var err error
		if table.createVS {
			vs, err := router.istioClient.NetworkingV1alpha3().VirtualServices(table.canary.Namespace).Get(context.TODO(), table.canary.Name, metav1.GetOptions{})
			require.NoError(t, err)

			if vs.Annotations == nil {
				vs.Annotations = make(map[string]string)
			}

			switch table.annotation {
			case "flagger":
				b, err := json.Marshal(table.spec)
				require.NoError(t, err)
				vs.Annotations[configAnnotation] = string(b)
			case "kubectl":
				vs.Annotations[kubectlAnnotation] = `{"apiVersion": "networking.istio.io/v1alpha3","kind": "VirtualService","metadata": {"annotations": {},"name": "podinfo","namespace": "test"},  "spec": {"gateways": ["ingressgateway.istio-system.svc.cluster.local"],"hosts": ["podinfo"],"http": [{"route": [{"destination": {"host": "podinfo"}}]}]}}`
			}
			_, err = router.istioClient.NetworkingV1alpha3().VirtualServices(table.canary.Namespace).Update(context.TODO(), vs, metav1.UpdateOptions{})
			require.NoError(t, err)
		}

		if table.callReconcile {
			err = router.Reconcile(table.canary)
			require.NoError(t, err)
		}

		err = router.Finalize(table.canary)
		if table.shouldError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}

		if table.spec != nil {
			vs, err := router.istioClient.NetworkingV1alpha3().VirtualServices(table.canary.Namespace).Get(context.TODO(), table.canary.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, *table.spec, vs.Spec)
		}
	}
}

func TestIstioRouter_Match(t *testing.T) {
	mocks := newFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// service.match is not exists, analysis match is exists
	err := router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	// test insert
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)
	assert.Len(t, vs.Spec.Http[0].Match, 1) // check for abtest-canary
	require.Equal(t, vs.Spec.Http[0].Match[0].Headers["x-user-type"].Exact, "test")
	assert.Len(t, vs.Spec.Http[1].Match, 0) // check for abtest-primary

	// Test Case that is service.match exists and multiple analysis.match
	mocks.abtest.Spec.Service.Match = []istiov1alpha3.HTTPMatchRequest{
		{
			Name: "podinfo",
			Uri: &istiov1alpha1.StringMatch{
				Prefix: "/podinfo",
			},
			Method: &istiov1alpha1.StringMatch{
				Exact: "GET",
			},
			IgnoreUriCase: true,
		},
	}
	mocks.abtest.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
		{
			Headers: map[string]istiov1alpha1.StringMatch{
				"x-user-type": {
					Exact: "test",
				},
				"x-auth-test": {
					Exact: "test",
				},
			},
		},
		{
			Headers: map[string]istiov1alpha1.StringMatch{
				"x-session-id": {
					Exact: "test",
				},
			},
		},
	}

	// apply changes
	err = router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)
	assert.Len(t, vs.Spec.Http[0].Match, 2) // check for abtest-canary
	require.Equal(t, vs.Spec.Http[0].Match[0].Uri.Prefix, "/podinfo")
	require.Equal(t, vs.Spec.Http[0].Match[0].Headers["x-user-type"].Exact, "test")
	require.Equal(t, vs.Spec.Http[0].Match[0].Headers["x-auth-test"].Exact, "test")
	require.Equal(t, vs.Spec.Http[0].Match[1].Uri.Prefix, "/podinfo")
	require.Equal(t, vs.Spec.Http[0].Match[1].Headers["x-session-id"].Exact, "test")
	assert.Len(t, vs.Spec.Http[1].Match, 1) // check for abtest-primary
	require.Equal(t, vs.Spec.Http[1].Match[0].Uri.Prefix, "/podinfo")
}

func TestRouteNameIstioRouter_Sync(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// test insert
	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	require.Len(t, vs.Spec.Http[0].Route, 2)

	// test update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Hosts
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Hosts = hosts
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cdClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Hosts, 2)

	// test drift
	vsClone := vs.DeepCopy()
	gateways := vsClone.Spec.Gateways
	gateways = append(gateways, "test-gateway.istio-system")
	vsClone.Spec.Gateways = gateways
	totalGateways := len(mocks.canary.Spec.Service.Gateways)

	vsGateways, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Update(context.TODO(), vsClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	totalGateways++
	assert.Len(t, vsGateways.Spec.Gateways, totalGateways)

	// undo change
	totalGateways--
	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Gateways, totalGateways)
}

func TestRouteNameIstioRouter_SetRoutes(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	pHost := fmt.Sprintf("%s-primary", mocks.canary.Spec.TargetRef.Name)
	cHost := fmt.Sprintf("%s-canary", mocks.canary.Spec.TargetRef.Name)

	t.Run("normal", func(t *testing.T) {
		p, c := 60, 40
		err := router.SetRoutes(mocks.canary, p, c, false)
		require.NoError(t, err)

		vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		var pRoute, cRoute istiov1alpha3.HTTPRouteDestination
		var mirror *istiov1alpha3.Destination
		for _, http := range vs.Spec.Http {
			for _, route := range http.Route {
				if route.Destination.Host == pHost {
					pRoute = route
				}
				if route.Destination.Host == cHost {
					cRoute = route
					mirror = http.Mirror
				}
			}
		}

		assert.Equal(t, p, pRoute.Weight)
		assert.Equal(t, c, cRoute.Weight)
		assert.Nil(t, mirror)

	})

	t.Run("session affinity", func(t *testing.T) {
		canary := mocks.canary.DeepCopy()
		cookieKey := "flagger-cookie"
		// enable session affinity and start canary run
		canary.Spec.Analysis.SessionAffinity = &v1beta1.SessionAffinity{
			CookieName: cookieKey,
			MaxAge:     300,
		}
		err := router.SetRoutes(canary, 0, 10, false)

		vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Len(t, vs.Spec.Http, 2)
		stickyRoute := vs.Spec.Http[0]
		weightedRoute := vs.Spec.Http[1]

		// stickyRoute should match against a cookie and direct all traffic to the canary when a canary run is active.
		var found bool
		for _, match := range stickyRoute.Match {
			if val, ok := match.Headers[cookieHeader]; ok {
				found = true
				assert.True(t, strings.Contains(val.Regex, cookieKey))
				for _, routeDest := range stickyRoute.Route {
					if routeDest.Destination.Host == pHost {
						assert.Equal(t, 0, routeDest.Weight)
					}
					if routeDest.Destination.Host == cHost {
						assert.Equal(t, 100, routeDest.Weight)
					}
				}
			}
		}
		assert.True(t, found)

		// weightedRoute should do regular weight based routing and inject the Set-Cookie header
		// for all responses returned from the canary deployment.
		for _, routeDest := range weightedRoute.Route {
			if routeDest.Destination.Host == pHost {
				assert.Equal(t, 0, routeDest.Weight)
			}
			if routeDest.Destination.Host == cHost {
				assert.Equal(t, 10, routeDest.Weight)
				val, ok := routeDest.Headers.Response.Add[setCookieHeader]
				assert.True(t, ok)
				assert.True(t, strings.HasPrefix(val, cookieKey))
				assert.True(t, strings.Contains(val, "Max-Age=300"))
			}
		}
		assert.True(t, strings.HasPrefix(canary.Status.SessionAffinityCookie, cookieKey))

		// reconcile canary, destination rules, virtual services
		err = router.Reconcile(canary)
		require.NoError(t, err)

		reconciledVS, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		// routes should not be changed.
		assert.Len(t, vs.Spec.Http, 2)
		assert.NotNil(t, reconciledVS)
		assert.Equal(t, cmp.Diff(reconciledVS.Spec.Http[0], stickyRoute), "")
		assert.Equal(t, cmp.Diff(reconciledVS.Spec.Http[1], weightedRoute), "")

		// further continue the canary run
		err = router.SetRoutes(canary, 50, 50, false)
		require.NoError(t, err)

		vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Len(t, vs.Spec.Http, 2)
		stickyRoute = vs.Spec.Http[0]
		weightedRoute = vs.Spec.Http[1]

		found = false
		for _, match := range stickyRoute.Match {
			if val, ok := match.Headers[cookieHeader]; ok {
				found = true
				assert.True(t, strings.Contains(val.Regex, cookieKey))
				for _, routeDest := range stickyRoute.Route {
					if routeDest.Destination.Host == pHost {
						assert.Equal(t, 0, routeDest.Weight)
					}
					if routeDest.Destination.Host == cHost {
						assert.Equal(t, 100, routeDest.Weight)
					}
				}
			}
		}
		assert.True(t, found)

		for _, routeDest := range weightedRoute.Route {
			if routeDest.Destination.Host == pHost {
				assert.Equal(t, 50, routeDest.Weight)
			}
			if routeDest.Destination.Host == cHost {
				assert.Equal(t, 50, routeDest.Weight)
				val, ok := routeDest.Headers.Response.Add[setCookieHeader]
				assert.True(t, ok)
				assert.True(t, strings.HasPrefix(val, cookieKey))
				assert.True(t, strings.Contains(val, "Max-Age=300"))
			}
		}
		assert.True(t, strings.HasPrefix(canary.Status.SessionAffinityCookie, cookieKey))
		sessionAffinityCookie := canary.Status.SessionAffinityCookie

		// promotion
		err = router.SetRoutes(canary, 100, 0, false)
		require.NoError(t, err)

		vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Len(t, vs.Spec.Http, 2)
		stickyRoute = vs.Spec.Http[0]
		weightedRoute = vs.Spec.Http[1]

		found = false
		for _, match := range stickyRoute.Match {
			if val, ok := match.Headers[cookieHeader]; ok {
				found = true
				assert.True(t, strings.Contains(val.Regex, cookieKey))
				for _, routeDest := range stickyRoute.Route {
					if routeDest.Destination.Host == pHost {
						assert.Equal(t, 100, routeDest.Weight)
					}
					if routeDest.Destination.Host == cHost {
						assert.Equal(t, 0, routeDest.Weight)
					}
				}
			}
		}
		assert.True(t, found)

		assert.Equal(t, canary.Status.SessionAffinityCookie, "")
		assert.Equal(t, canary.Status.PreviousSessionAffinityCookie, sessionAffinityCookie)

		val, ok := stickyRoute.Headers.Response.Add[setCookieHeader]
		assert.True(t, ok)
		assert.True(t, strings.HasPrefix(val, sessionAffinityCookie))
		assert.True(t, strings.Contains(val, "Max-Age=-1"))

		// delete the Set-Cookie header from responses returned by the weighted route
		for _, routeDest := range weightedRoute.Route {
			if routeDest.Destination.Host == pHost {
				assert.Equal(t, 100, routeDest.Weight)
			}
			if routeDest.Destination.Host == cHost {
				assert.Equal(t, 0, routeDest.Weight)
				if routeDest.Headers != nil && routeDest.Headers.Response != nil {
					_, ok := routeDest.Headers.Response.Add[setCookieHeader]
					assert.False(t, ok)
				}
			}
		}
	})

	t.Run("mirror", func(t *testing.T) {
		for _, w := range []int{0, 10, 50} {
			p, c := 100, 0

			// set mirror weight
			mocks.canary.Spec.Analysis.MirrorWeight = w
			err := router.SetRoutes(mocks.canary, p, c, true)
			require.NoError(t, err)

			vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
			require.NoError(t, err)

			var pRoute, cRoute istiov1alpha3.HTTPRouteDestination
			var mirror *istiov1alpha3.Destination
			var mirrorWeight *istiov1alpha3.Percent
			for _, http := range vs.Spec.Http {
				for _, route := range http.Route {
					if route.Destination.Host == pHost {
						pRoute = route
					}
					if route.Destination.Host == cHost {
						cRoute = route
						mirror = http.Mirror
						mirrorWeight = http.MirrorPercentage
					}
				}
			}

			assert.Equal(t, p, pRoute.Weight)
			assert.Equal(t, c, cRoute.Weight)
			if assert.NotNil(t, mirror) {
				assert.Equal(t, cHost, mirror.Host)
			}

			if w > 0 && assert.NotNil(t, mirrorWeight) {
				assert.Equal(t, w, int(mirrorWeight.Value))
			} else {
				assert.Nil(t, mirrorWeight)
			}
		}
	})
}

func TestIstioRouteNameRouter_GetRoutes(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.False(t, m)

	mocks.canary = newTestMirror()

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	p, c, m, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)

	// A Canary resource with mirror on does not automatically create mirroring
	// in the virtual server (mirroring is activated as a temporary stage).
	assert.False(t, m)

	// Adjust vs to activate mirroring.
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cHost := fmt.Sprintf("%s-canary", mocks.canary.Spec.TargetRef.Name)
	for i, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == cHost {
				vs.Spec.Http[i].Mirror = &istiov1alpha3.Destination{
					Host: cHost,
				}
			}
		}
	}
	_, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices(mocks.canary.Namespace).Update(context.TODO(), vs, metav1.UpdateOptions{})
	require.NoError(t, err)

	p, c, m, err = router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, p)
	assert.Equal(t, 0, c)
	assert.True(t, m)
}

func TestIstioRouteNameRouter_HTTPRequestHeaders(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	assert.Equal(t, "15000", vs.Spec.Http[0].Headers.Request.Add["x-envoy-upstream-rq-timeout-ms"])
	assert.Equal(t, "test", vs.Spec.Http[0].Headers.Request.Remove[0])
	assert.Equal(t, "token", vs.Spec.Http[0].Headers.Response.Remove[0])
}

func TestIstioRouteNameRouter_CORS(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	assert.NotNil(t, vs.Spec.Http[0].CorsPolicy)
	assert.Len(t, vs.Spec.Http[0].CorsPolicy.AllowMethods, 2)
}

func TestIstioRouteNameRouter_ABTest(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	// test insert
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)

	p := 0
	c := 100
	m := false

	err = router.SetRoutes(mocks.abtest, p, c, m)
	require.NoError(t, err)

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)

	pHost := fmt.Sprintf("%s-primary", mocks.abtest.Spec.TargetRef.Name)
	cHost := fmt.Sprintf("%s-canary", mocks.abtest.Spec.TargetRef.Name)
	pRoute := istiov1alpha3.HTTPRouteDestination{}
	cRoute := istiov1alpha3.HTTPRouteDestination{}
	var mirror *istiov1alpha3.Destination

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == pHost {
				pRoute = route
			}
			if route.Destination.Host == cHost {
				cRoute = route
				mirror = http.Mirror
			}
		}
	}

	assert.Equal(t, p, pRoute.Weight)
	assert.Equal(t, c, cRoute.Weight)
	assert.Nil(t, mirror)
}

func TestIstioRouteNameRouter_GatewayPort(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	port := vs.Spec.Http[0].Route[0].Destination.Port.Number
	assert.Equal(t, uint32(mocks.canary.Spec.Service.Port), port)
}

func TestIstioRouteNameRouter_Delegate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		mocks := newRouteNameFixture(nil)
		mocks.canary.Spec.Service.Hosts = []string{}
		mocks.canary.Spec.Service.Gateways = []string{}
		mocks.canary.Spec.Service.Delegation = true

		router := &IstioRouter{
			logger:        mocks.logger,
			flaggerClient: mocks.flaggerClient,
			istioClient:   mocks.meshClient,
			kubeClient:    mocks.kubeClient,
		}

		err := router.Reconcile(mocks.canary)
		require.NoError(t, err)

		vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Equal(t, 0, len(vs.Spec.Hosts))
		assert.Equal(t, 0, len(vs.Spec.Gateways))

		port := vs.Spec.Http[0].Route[0].Destination.Port.Number
		assert.Equal(t, uint32(mocks.canary.Spec.Service.Port), port)
	})

	t.Run("invalid", func(t *testing.T) {
		mocks := newFixture(nil)
		if len(mocks.canary.Spec.Service.Gateways) == 0 {
			// in this case, the gateways or hosts should not be not empty because it requires to cause an error.
			mocks.canary.Spec.Service.Gateways = []string{
				"public-gateway.istio",
				"mesh",
			}
		}
		mocks.canary.Spec.Service.Delegation = true

		router := &IstioRouter{
			logger:        mocks.logger,
			flaggerClient: mocks.flaggerClient,
			istioClient:   mocks.meshClient,
			kubeClient:    mocks.kubeClient,
		}

		err := router.Reconcile(mocks.canary)
		require.Error(t, err)
	})
}

func TestIstioRouteNameRouter_Finalize(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	flaggerSpec := &istiov1alpha3.VirtualServiceSpec{
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match:      mocks.canary.Spec.Service.Match,
				Rewrite:    mocks.canary.Spec.Service.Rewrite,
				Timeout:    mocks.canary.Spec.Service.Timeout,
				Retries:    mocks.canary.Spec.Service.Retries,
				CorsPolicy: mocks.canary.Spec.Service.CorsPolicy,
			},
		},
	}

	kubectlSpec := &istiov1alpha3.VirtualServiceSpec{
		Hosts:    []string{"podinfo"},
		Gateways: []string{"ingressgateway.istio-system.svc.cluster.local"},
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match: nil,
				Route: []istiov1alpha3.HTTPRouteDestination{
					{
						Destination: istiov1alpha3.Destination{Host: "podinfo"},
					},
				},
			},
		},
	}

	tables := []struct {
		router        *IstioRouter
		spec          *istiov1alpha3.VirtualServiceSpec
		shouldError   bool
		createVS      bool
		canary        *v1beta1.Canary
		callReconcile bool
		annotation    string
	}{
		// VS not found
		{router: router, spec: nil, shouldError: true, createVS: false, canary: mocks.canary, callReconcile: false, annotation: ""},
		// No annotation found but still finalizes
		{router: router, spec: nil, shouldError: false, createVS: false, canary: mocks.canary, callReconcile: true, annotation: ""},
		// Spec should match annotation after finalize
		{router: router, spec: flaggerSpec, shouldError: false, createVS: true, canary: mocks.canary, callReconcile: true, annotation: "flagger"},
		// Need to test kubectl annotation
		{router: router, spec: kubectlSpec, shouldError: false, createVS: true, canary: mocks.canary, callReconcile: true, annotation: "kubectl"},
	}

	for _, table := range tables {
		var err error
		if table.createVS {
			vs, err := router.istioClient.NetworkingV1alpha3().VirtualServices(table.canary.Namespace).Get(context.TODO(), table.canary.Name, metav1.GetOptions{})
			require.NoError(t, err)

			if vs.Annotations == nil {
				vs.Annotations = make(map[string]string)
			}

			switch table.annotation {
			case "flagger":
				b, err := json.Marshal(table.spec)
				require.NoError(t, err)
				vs.Annotations[configAnnotation] = string(b)
			case "kubectl":
				vs.Annotations[kubectlAnnotation] = `{"apiVersion": "networking.istio.io/v1alpha3","kind": "VirtualService","metadata": {"annotations": {},"name": "podinfo","namespace": "test"},  "spec": {"gateways": ["ingressgateway.istio-system.svc.cluster.local"],"hosts": ["podinfo"],"http": [{"route": [{"destination": {"host": "podinfo"}}]}]}}`
			}
			_, err = router.istioClient.NetworkingV1alpha3().VirtualServices(table.canary.Namespace).Update(context.TODO(), vs, metav1.UpdateOptions{})
			require.NoError(t, err)
		}

		if table.callReconcile {
			err = router.Reconcile(table.canary)
			require.NoError(t, err)
		}

		err = router.Finalize(table.canary)
		if table.shouldError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}

		if table.spec != nil {
			vs, err := router.istioClient.NetworkingV1alpha3().VirtualServices(table.canary.Namespace).Get(context.TODO(), table.canary.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, *table.spec, vs.Spec)
		}
	}
}

func TestIstioRouteNameRouter_Match(t *testing.T) {
	mocks := newRouteNameFixture(nil)
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	// service.match is not exists, analysis match is exists
	err := router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	// test insert
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)
	assert.Len(t, vs.Spec.Http[0].Match, 1) // check for abtest-canary
	require.Equal(t, vs.Spec.Http[0].Match[0].Headers["x-user-type"].Exact, "test")
	assert.Len(t, vs.Spec.Http[1].Match, 0) // check for abtest-primary

	// Test Case that is service.match exists and multiple analysis.match
	mocks.abtest.Spec.Service.Match = []istiov1alpha3.HTTPMatchRequest{
		{
			Name: "podinfo",
			Uri: &istiov1alpha1.StringMatch{
				Prefix: "/podinfo",
			},
			Method: &istiov1alpha1.StringMatch{
				Exact: "GET",
			},
			IgnoreUriCase: true,
		},
	}
	mocks.abtest.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
		{
			Headers: map[string]istiov1alpha1.StringMatch{
				"x-user-type": {
					Exact: "test",
				},
				"x-auth-test": {
					Exact: "test",
				},
			},
		},
		{
			Headers: map[string]istiov1alpha1.StringMatch{
				"x-session-id": {
					Exact: "test",
				},
			},
		},
	}

	// apply changes
	err = router.Reconcile(mocks.abtest)
	require.NoError(t, err)

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)
	assert.Len(t, vs.Spec.Http[0].Match, 2) // check for abtest-canary
	require.Equal(t, vs.Spec.Http[0].Match[0].Uri.Prefix, "/podinfo")
	require.Equal(t, vs.Spec.Http[0].Match[0].Headers["x-user-type"].Exact, "test")
	require.Equal(t, vs.Spec.Http[0].Match[0].Headers["x-auth-test"].Exact, "test")
	require.Equal(t, vs.Spec.Http[0].Match[1].Uri.Prefix, "/podinfo")
	require.Equal(t, vs.Spec.Http[0].Match[1].Headers["x-session-id"].Exact, "test")
	assert.Len(t, vs.Spec.Http[1].Match, 1) // check for abtest-primary
	require.Equal(t, vs.Spec.Http[1].Match[0].Uri.Prefix, "/podinfo")
}
