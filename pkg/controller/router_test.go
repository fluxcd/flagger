package controller

import (
	"fmt"
	"testing"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCanaryRouter_Sync(t *testing.T) {
	mocks := SetupMocks()
	err := mocks.router.Sync(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if canarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got svc port name %s wanted %s", canarySvc.Spec.Ports[0].Name, "http")
	}

	if canarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got svc port %v wanted %v", canarySvc.Spec.Ports[0].Port, 9898)
	}

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if primarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got primary svc port name %s wanted %s", primarySvc.Spec.Ports[0].Name, "http")
	}

	if primarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got primary svc port %v wanted %v", primarySvc.Spec.Ports[0].Port, 9898)
	}

	vs, err := mocks.istioClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(vs.Spec.Http) != 1 {
		t.Errorf("Got Istio VS Http %v wanted %v", len(vs.Spec.Http), 1)
	}

	if len(vs.Spec.Http[0].Route) != 2 {
		t.Errorf("Got Istio VS routes %v wanted %v", len(vs.Spec.Http[0].Route), 2)
	}
}

func TestCanaryRouter_GetRoutes(t *testing.T) {
	mocks := SetupMocks()
	err := mocks.router.Sync(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p.Weight != 100 {
		t.Errorf("Got primary weight %v wanted %v", p.Weight, 100)
	}

	if c.Weight != 0 {
		t.Errorf("Got canary weight %v wanted %v", c.Weight, 0)
	}
}

func TestCanaryRouter_SetRoutes(t *testing.T) {
	mocks := SetupMocks()
	err := mocks.router.Sync(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p.Weight = 50
	c.Weight = 50

	err = mocks.router.SetRoutes(mocks.canary, p, c)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err := mocks.istioClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	pRoute := istiov1alpha3.DestinationWeight{}
	cRoute := istiov1alpha3.DestinationWeight{}

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == fmt.Sprintf("%s-primary", mocks.canary.Spec.TargetRef.Name) {
				pRoute = route
			}
			if route.Destination.Host == mocks.canary.Spec.TargetRef.Name {
				cRoute = route
			}
		}
	}

	if pRoute.Weight != p.Weight {
		t.Errorf("Got primary weight %v wanted %v", pRoute.Weight, c.Weight)
	}

	if cRoute.Weight != c.Weight {
		t.Errorf("Got canary weight %v wanted %v", cRoute.Weight, c.Weight)
	}
}
