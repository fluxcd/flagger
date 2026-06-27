package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/flagger/pkg/router"
)

func TestController_MultiClusterFinalizer(t *testing.T) {
	// 1. Setup Canary WITHOUT RevertOnDeletion
	canary := newDeploymentTestCanary()
	canary.Spec.RevertOnDeletion = false

	// 2. Setup Fixture with Multi-Cluster ENABLED
	// We'll mock the ClusterManager directly in the factory
	mocks := newDeploymentFixture(canary)

	// Inject ClusterManager to simulate multi-cluster enabled
	cm := router.NewClusterManager(mocks.kubeClient, mocks.flaggerClient, mocks.logger, "istio/multiCluster=true", "istio-system")
	rf := router.NewFactory(nil, mocks.kubeClient, mocks.flaggerClient, nil, "", "", mocks.logger, mocks.flaggerClient, true, cm)

	mocks.ctrl.routerFactory = rf // Override with our multi-cluster factory

	// 3. Sync the canary - this should ADD the finalizer because multi-cluster is enabled
	key := fmt.Sprintf("%s/%s", canary.Namespace, canary.Name)
	err := mocks.ctrl.syncHandler(key)
	require.NoError(t, err)

	// 4. Verify Finalizer was added
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(context.Background(), canary.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, hasFinalizer(c), "Finalizer should be added in multi-cluster mode even if RevertOnDeletion is false")

	// Initialize canary (creates primary, set status etc)
	mocks.ctrl.advanceCanary(canary.Name, canary.Namespace)
	mocks.makePrimaryReady(t)
	mocks.makeCanaryReady(t)
	mocks.ctrl.advanceCanary(canary.Name, canary.Namespace)

	// Update informer indexer
	c, _ = mocks.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(context.Background(), canary.Name, metav1.GetOptions{})
	err = mocks.ctrl.flaggerInformers.CanaryInformer.Informer().GetIndexer().Update(c)
	require.NoError(t, err)

	// 5. Mark for Deletion
	now := metav1.Now()
	c.DeletionTimestamp = &now
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Update(context.Background(), c, metav1.UpdateOptions{})
	require.NoError(t, err)
	// Update informer indexer
	err = mocks.ctrl.flaggerInformers.CanaryInformer.Informer().GetIndexer().Update(c)
	require.NoError(t, err)

	// 6. Sync again - this should TRIGGER finalization
	err = mocks.ctrl.syncHandler(key)
	require.NoError(t, err)

	// 7. Verify Finalizer was removed (after successful finalization)
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(context.Background(), canary.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.False(t, hasFinalizer(c), "Finalizer should be removed after successful finalization")
}
