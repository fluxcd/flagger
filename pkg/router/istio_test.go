package router

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
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

		var pRoute, cRoute istiov1alpha3.DestinationWeight
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

	t.Run("mirror", func(t *testing.T) {
		for _, w := range []int{0, 10, 50} {
			p, c := 100, 0

			// set mirror weight
			mocks.canary.Spec.Analysis.MirrorWeight = w
			err := router.SetRoutes(mocks.canary, p, c, true)
			require.NoError(t, err)

			vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
			require.NoError(t, err)

			var pRoute, cRoute istiov1alpha3.DestinationWeight
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
	pRoute := istiov1alpha3.DestinationWeight{}
	cRoute := istiov1alpha3.DestinationWeight{}
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
				Route: []istiov1alpha3.DestinationWeight{
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
