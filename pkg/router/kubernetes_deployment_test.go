package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceRouter_Create(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDeploymentRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "http", canarySvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9898), canarySvc.Spec.Ports[0].Port)

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "http", primarySvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9898), primarySvc.Spec.Ports[0].Port)
}

func TestServiceRouter_Update(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDeploymentRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	canaryClone := canary.DeepCopy()
	canaryClone.Spec.Service.PortName = "grpc"

	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(canaryClone)
	require.NoError(t, err)

	// apply changes
	err = router.Initialize(c)
	require.NoError(t, err)
	err = router.Reconcile(c)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "grpc", canarySvc.Spec.Ports[0].Name)
}

func TestServiceRouter_Undo(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDeploymentRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	svcClone := canarySvc.DeepCopy()
	svcClone.Spec.Ports[0].Name = "http2-podinfo"
	svcClone.Spec.Ports[0].Port = 8080

	_, err = mocks.kubeClient.CoreV1().Services("default").Update(svcClone)
	require.NoError(t, err)

	// undo changes
	err = router.Initialize(mocks.canary)
	require.NoError(t, err)
	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err = mocks.kubeClient.CoreV1().Services("default").Get("podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "http", canarySvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9898), canarySvc.Spec.Ports[0].Port)
}
