package router

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get("podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get("podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, vs.Spec.Http, 1)
	require.Len(t, vs.Spec.Http[0].Route, 2)

	// test update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Hosts
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Hosts = hosts
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cdClone)
	require.NoError(t, err)

	// apply change
	err = router.Reconcile(canary)
	require.NoError(t, err)

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Hosts, 2)

	// test drift
	vsClone := vs.DeepCopy()
	gateways := vsClone.Spec.Gateways
	gateways = append(gateways, "test-gateway.istio-system")
	vsClone.Spec.Gateways = gateways
	totalGateways := len(mocks.canary.Spec.Service.Gateways)

	vsGateways, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Update(vsClone)
	require.NoError(t, err)

	totalGateways++
	assert.Len(t, vsGateways.Spec.Gateways, totalGateways)

	// undo change
	totalGateways--
	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
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

	p, c, m, err := router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	p = 60
	c = 40
	m = false

	err = router.SetRoutes(mocks.canary, p, c, m)
	require.NoError(t, err)

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	pHost := fmt.Sprintf("%s-primary", mocks.canary.Spec.TargetRef.Name)
	cHost := fmt.Sprintf("%s-canary", mocks.canary.Spec.TargetRef.Name)
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

	mirror = nil
	p = 100
	c = 0
	m = true

	err = router.SetRoutes(mocks.canary, p, c, m)
	require.NoError(t, err)

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)

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
	if assert.NotNil(t, mirror) {
		assert.Equal(t, cHost, mirror.Host)
	}
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
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
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
	_, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices(mocks.canary.Namespace).Update(vs)
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

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
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

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
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
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("abtest", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, vs.Spec.Http, 2)

	p := 0
	c := 100
	m := false

	err = router.SetRoutes(mocks.abtest, p, c, m)
	require.NoError(t, err)

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("abtest", metav1.GetOptions{})
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

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	port := vs.Spec.Http[0].Route[0].Destination.Port.Number
	assert.Equal(t, uint32(mocks.canary.Spec.Service.Port), port)
}
