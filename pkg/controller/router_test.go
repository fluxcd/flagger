package controller

import (
	"fmt"
	"testing"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	fakeIstio "github.com/knative/pkg/client/clientset/versioned/fake"
	fakeFlagger "github.com/stefanprodan/flagger/pkg/client/clientset/versioned/fake"
	"github.com/stefanprodan/flagger/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCanaryRouter_Sync(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")

	router := &CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}

	err := router.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	canarySvc, err := kubeClient.CoreV1().Services("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if canarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got svc port name %s wanted %s", canarySvc.Spec.Ports[0].Name, "http")
	}

	if canarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got svc port %v wanted %v", canarySvc.Spec.Ports[0].Port, 9898)
	}

	primarySvc, err := kubeClient.CoreV1().Services("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if primarySvc.Spec.Ports[0].Name != "http" {
		t.Errorf("Got primary svc port name %s wanted %s", primarySvc.Spec.Ports[0].Name, "http")
	}

	if primarySvc.Spec.Ports[0].Port != 9898 {
		t.Errorf("Got primary svc port %v wanted %v", primarySvc.Spec.Ports[0].Port, 9898)
	}

	vs, err := istioClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
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
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")

	router := &CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}

	err := router.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := router.GetRoutes(canary)
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
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")

	router := &CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}

	err := router.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	p.Weight = 50
	c.Weight = 50

	err = router.SetRoutes(canary, p, c)
	if err != nil {
		t.Fatal(err.Error())
	}

	vs, err := istioClient.NetworkingV1alpha3().VirtualServices("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	pRoute := istiov1alpha3.DestinationWeight{}
	cRoute := istiov1alpha3.DestinationWeight{}

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name) {
				pRoute = route
			}
			if route.Destination.Host == canary.Spec.TargetRef.Name {
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
