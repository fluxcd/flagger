package router

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// check canary virtual service
	vsCanaryName := fmt.Sprintf("%s-canary.%s", mocks.appmeshCanary.Spec.TargetRef.Name, mocks.appmeshCanary.Namespace)
	vsCanary, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(vsCanaryName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// check if the canary virtual service routes all traffic to the canary virtual node
	target := vsCanary.Spec.Routes[0].Http.Action.WeightedTargets[0]
	canaryVirtualNodeName := fmt.Sprintf("%s-canary", mocks.appmeshCanary.Spec.TargetRef.Name)
	if target.VirtualNodeName != canaryVirtualNodeName {
		t.Errorf("Got VirtualNodeName %v wanted %v", target.VirtualNodeName, canaryVirtualNodeName)
	}
	if target.Weight != 100 {
		t.Errorf("Got weight %v wanted %v", target.Weight, 100)
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
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("appmesh", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	cdClone := cd.DeepCopy()
	hosts := cdClone.Spec.Service.Backends
	hosts = append(hosts, "test.example.com")
	cdClone.Spec.Service.Backends = hosts
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
		t.Errorf("Got weight %v wanted %v", weight, 50)
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

	err = router.SetRoutes(mocks.appmeshCanary, 60, 40, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	p, c, m, err := router.GetRoutes(mocks.appmeshCanary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if p != 60 {
		t.Errorf("Got primary weight %v wanted %v", p, 60)
	}

	if c != 40 {
		t.Errorf("Got canary weight %v wanted %v", c, 40)
	}

	if m != false {
		t.Errorf("Got mirror %v wanted %v", m, false)
	}
}

func TestAppmeshRouter_ABTest(t *testing.T) {
	mocks := setupfakeClients()
	router := &AppMeshRouter{
		logger:        mocks.logger,
		flaggerClient: mocks.flaggerClient,
		appmeshClient: mocks.meshClient,
		kubeClient:    mocks.kubeClient,
	}

	err := router.Reconcile(mocks.abtest)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check virtual service
	vsName := fmt.Sprintf("%s.%s", mocks.abtest.Spec.TargetRef.Name, mocks.abtest.Namespace)
	vs, err := router.appmeshClient.AppmeshV1beta1().VirtualServices("default").Get(vsName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// check virtual service
	if len(vs.Spec.Routes) != 2 {
		t.Errorf("Got routes %v wanted %v", len(vs.Spec.Routes), 2)
	}

	// check headers
	if len(vs.Spec.Routes[0].Http.Match.Headers) < 1 {
		t.Errorf("Got no http match headers")
	}

	header := vs.Spec.Routes[0].Http.Match.Headers[0].Name
	if header != "x-user-type" {
		t.Errorf("Got http match header %v wanted %v", header, "x-user-type")
	}

	exactMatch := *vs.Spec.Routes[0].Http.Match.Headers[0].Match.Exact
	if exactMatch != "test" {
		t.Errorf("Got http match header exact %v wanted %v", exactMatch, "test")
	}
}

func TestAppmeshRouter_Gateway(t *testing.T) {
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

	expose := vs.Annotations["gateway.appmesh.k8s.aws/expose"]
	if expose != "true" {
		t.Errorf("Got gateway expose annotation %v wanted %v", expose, "true")
	}

	domain := vs.Annotations["gateway.appmesh.k8s.aws/domain"]
	if !strings.Contains(domain, mocks.appmeshCanary.Spec.Service.Hosts[0]) {
		t.Errorf("Got gateway domain annotation %v wanted %v", domain, mocks.appmeshCanary.Spec.Service.Hosts[0])
	}

	timeout := vs.Annotations["gateway.appmesh.k8s.aws/timeout"]
	if timeout != mocks.appmeshCanary.Spec.Service.Timeout {
		t.Errorf("Got gateway timeout annotation %v wanted %v", timeout, mocks.appmeshCanary.Spec.Service.Timeout)
	}

	retries := vs.Annotations["gateway.appmesh.k8s.aws/retries"]
	if retries != strconv.Itoa(mocks.appmeshCanary.Spec.Service.Retries.Attempts) {
		t.Errorf("Got gateway retries annotation %v wanted %v", retries, strconv.Itoa(mocks.appmeshCanary.Spec.Service.Retries.Attempts))
	}
}
