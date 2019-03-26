package router

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestAppmeshRouter_Reconcile(t *testing.T) {
	mocks := setupfakeClients()
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.appmeshCanary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check virtual service
	vsName := fmt.Sprintf("%s.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	vs, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(vsName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	meshName := mocks.appmeshCanary.Spec.Service.MeshName
	if vs.Spec.MeshName != meshName {
		t.Errorf("Got mesh name %v wanted %v", vs.Spec.MeshName, meshName)
	}

	targetsCount := len(vs.Spec.Routes[0].Http.Action.WeightedTargets)
	if targetsCount != 2 {
		t.Errorf("Got routes %v wanted %v", targetsCount, 2)
	}

	// check virtual node
	vnName := mocks.appmeshCanary.Spec.TargetRef.Name
	vn, err := router.appmeshClient.AppmeshV1beta1().VirtualNodes("default").Get(vnName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryDNS := fmt.Sprintf("%s-primary.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	vnHostName := vn.Spec.ServiceDiscovery.Dns.HostName
	if vnHostName != primaryDNS {
		t.Errorf("Got DNS host name %v wanted %v", vnHostName, primaryDNS)
	}

	// test backends update
	cd, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Get("appmesh", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Backends
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Backends = hosts
	canary, err := mocks.flaggerClient.FlaggerV1alpha3().Canaries("default").Update(cdClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply change
	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify
	vnCanaryName := fmt.Sprintf("%s-canary", mocks.appmeshCanary.Spec.TargetRef.Name)
	vnCanary, err := router.appmeshClient.AppmeshV1beta1().VirtualNodes("default").Get(vnCanaryName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(vnCanary.Spec.Backends) != 2 {
		t.Errorf("Got backends %v wanted %v", len(vnCanary.Spec.Backends), 2)
	}

	// test weight update
	vsClone := vs.DeepCopy()
	vsClone.Spec.Routes[0].Http.Action.WeightedTargets[0].Weight = 50
	vsClone.Spec.Routes[0].Http.Action.WeightedTargets[1].Weight = 50
	vs, err = mocks.meshClient.AppmeshV1beta1().VirtualServices("default").Update(vsClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply change
	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
	vs, err = router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(vsName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	weight := vs.Spec.Routes[0].Http.Action.WeightedTargets[0].Weight
	if weight != 50 {
		t.Errorf("Got weight %v wanted %v", weight, 502)
	}

	// test URI update
	vsClone = vs.DeepCopy()
	vsClone.Spec.Routes[0].Http.Match.Prefix = "api"
	vs, err = mocks.meshClient.AppmeshV1beta1().VirtualServices("default").Update(vsClone)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply change
	err = router.Reconcile(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
	vs, err = router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(vsName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	prefix := vs.Spec.Routes[0].Http.Match.Prefix
	if prefix != "/" {
		t.Errorf("Got prefix %v wanted %v", prefix, "/")
	}
}

func TestAppmeshRouter_GetSetRoutes(t *testing.T) {
	mocks := setupfakeClients()
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.appmeshCanary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = router.SetRoutes(mocks.appmeshCanary, 60, 40)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, err := router.GetRoutes(mocks.appmeshCanary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 60 {
		t.Errorf("Got primary weight %v wanted %v", p, 60)
	}

	if c != 40 {
		t.Errorf("Got canary weight %v wanted %v", c, 40)
	}
}
