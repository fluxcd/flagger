package router

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
)

func TestIstioRouter_Sync(t *testing.T) {
	mocks := setupfakeClients()
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get("podinfo-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = mocks.meshClient.NetworkingV1alpha3().DestinationRules("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(vs.Spec.Http) != 1 {
		t.Errorf("Got Istio VS Http %v wanted %v", len(vs.Spec.Http), 1)
	}

	if len(vs.Spec.Http[0].Route) != 2 {
		t.Errorf("Got Istio VS routes %v wanted %v", len(vs.Spec.Http[0].Route), 2)
	}

	// test update
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Hosts
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Hosts = hosts
	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(cdClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply change
	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(vs.Spec.Hosts) != 2 {
		t.Errorf("Got Istio VS hosts %v wanted %v", vs.Spec.Hosts, 2)
	}

	// test drift
	vsClone := vs.DeepCopy()
	gateways := vsClone.Spec.Gateways
	gateways = append(gateways, "test-gateway.istio-system")
	vsClone.Spec.Gateways = gateways

	vsGateways, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Update(vsClone)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(vsGateways.Spec.Gateways) != 2 {
		t.Errorf("Got Istio VS gateway %v wanted %v", vsGateways.Spec.Gateways, 2)
	}

	// undo change
	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify
	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(vs.Spec.Gateways) != 1 {
		t.Errorf("Got Istio VS gateways %v wanted %v", vs.Spec.Gateways, 1)
	}
}

func TestIstioRouter_SetRoutes(t *testing.T) {
	mocks := setupfakeClients()
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err := router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p = 60
	c = 40
	m = false

	err = router.SetRoutes(mocks.canary, p, c, m)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	if pRoute.Weight != p {
		t.Errorf("Got primary weight %v wanted %v", pRoute.Weight, p)
	}

	if cRoute.Weight != c {
		t.Errorf("Got canary weight %v wanted %v", cRoute.Weight, c)
	}

	if mirror != nil {
		t.Errorf("Got mirror %v wanted nil", mirror)
	}

	mirror = nil
	p = 100
	c = 0
	m = true

	err = router.SetRoutes(mocks.canary, p, c, m)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	if pRoute.Weight != p {
		t.Errorf("Got primary weight %v wanted %v", pRoute.Weight, p)
	}

	if cRoute.Weight != c {
		t.Errorf("Got canary weight %v wanted %v", cRoute.Weight, c)
	}

	if mirror == nil {
		t.Errorf("Got mirror nil wanted a mirror")
	} else if mirror.Host != cHost {
		t.Errorf("Got mirror host \"%v\" wanted \"%v\"", mirror.Host, cHost)
	}
}

func TestIstioRouter_GetRoutes(t *testing.T) {
	mocks := setupfakeClients()
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err := router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 100 {
		t.Errorf("Got primary weight %v wanted %v", p, 100)
	}

	if c != 0 {
		t.Errorf("Got canary weight %v wanted %v", c, 0)
	}

	if m != false {
		t.Errorf("Got mirror %v wanted %v", m, false)
	}

	mocks.canary = newMockMirror()

	err = router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err = router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 100 {
		t.Errorf("Got primary weight %v wanted %v", p, 100)
	}

	if c != 0 {
		t.Errorf("Got canary weight %v wanted %v", c, 0)
	}

	// A Canary resource with mirror on does not automatically create mirroring
	// in the virtual server (mirroring is activated as a temporary stage).
	if m != false {
		t.Errorf("Got mirror %v wanted %v", m, false)
	}

	// Adjust vs to activate mirroring.
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

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
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err = router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 100 {
		t.Errorf("Got primary weight %v wanted %v", p, 100)
	}

	if c != 0 {
		t.Errorf("Got canary weight %v wanted %v", c, 0)
	}

	if m != true {
		t.Errorf("Got mirror %v wanted %v", m, true)
	}
}

func TestIstioRouter_HTTPRequestHeaders(t *testing.T) {
	mocks := setupfakeClients()
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(vs.Spec.Http) != 1 {
		t.Fatalf("Got HTTPRoute %v wanted %v", len(vs.Spec.Http), 1)
	}

	timeout := vs.Spec.Http[0].AppendHeaders["x-envoy-upstream-rq-timeout-ms"]
	if timeout != "15000" {
		t.Errorf("Got timeout %v wanted %v", timeout, "15000")
	}
}

func TestIstioRouter_CORS(t *testing.T) {
	mocks := setupfakeClients()
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(vs.Spec.Http) != 1 {
		t.Fatalf("Got HTTPRoute %v wanted %v", len(vs.Spec.Http), 1)
	}

	if vs.Spec.Http[0].CorsPolicy == nil {
		t.Fatal("Got not CORS policy")
	}

	methods := vs.Spec.Http[0].CorsPolicy.AllowMethods
	if len(methods) != 2 {
		t.Fatalf("Got CORS allow methods %v wanted %v", len(methods), 2)
	}
}

func TestIstioRouter_ABTest(t *testing.T) {
	mocks := setupfakeClients()
	router := &IstioRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		istioClient:   mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.abtest)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test insert
	vs, err := mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("abtest", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(vs.Spec.Http) != 2 {
		t.Errorf("Got Istio VS Http %v wanted %v", len(vs.Spec.Http), 2)
	}

	p := 0
	c := 100
	m := false

	err = router.SetRoutes(mocks.abtest, p, c, m)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err = mocks.meshClient.NetworkingV1alpha3().VirtualServices("default").Get("abtest", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

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

	if pRoute.Weight != p {
		t.Errorf("Got primary weight %v wanted %v", pRoute.Weight, p)
	}

	if cRoute.Weight != c {
		t.Errorf("Got canary weight %v wanted %v", cRoute.Weight, c)
	}

	if mirror != nil {
		t.Errorf("Got mirror %v wanted nil", mirror)
	}
}
