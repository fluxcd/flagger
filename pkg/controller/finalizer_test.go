package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTesting "k8s.io/client-go/testing"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
)

func TestFinalizer_hasFinalizer(t *testing.T) {
	c := newDeploymentTestCanary()
	require.False(t, hasFinalizer(c))

	c.Finalizers = append(c.Finalizers, finalizer)
	require.True(t, hasFinalizer(c))
}

func TestFinalizer_addFinalizer(t *testing.T) {

	cs := fakeFlagger.NewSimpleClientset(newDeploymentTestCanary())
	// prepend so it is evaluated over the catch all *
	cs.PrependReactor("update", "canaries", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("failed to add finalizer to canary %s", "testCanary")
	})

	m := fixture{
		canary:        newDeploymentTestCanary(),
		flaggerClient: cs,
		ctrl:          &Controller{flaggerClient: cs},
	}

	tables := []struct {
		mock   fixture
		canary *flaggerv1.Canary
		expErr bool
	}{
		{newDeploymentFixture(nil), newDeploymentTestCanary(), false},
		{m, m.canary, true},
	}

	for _, table := range tables {
		err := table.mock.ctrl.addFinalizer(table.canary)

		if table.expErr {
			require.NotNil(t, err)
		} else {
			require.Nil(t, err)
		}
	}
}

func TestFinalizer_removeFinalizer(t *testing.T) {

	withFinalizer := newDeploymentTestCanary()
	withFinalizer.Finalizers = append(withFinalizer.Finalizers, finalizer)

	cs := fakeFlagger.NewSimpleClientset(newDeploymentTestCanary())
	// prepend so it is evaluated over the catch all *
	cs.PrependReactor("update", "canaries", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("failed to add finalizer to canary %s", "testCanary")
	})
	m := fixture{
		canary:        withFinalizer,
		flaggerClient: cs,
		ctrl:          &Controller{flaggerClient: cs},
	}

	tables := []struct {
		mock   fixture
		canary *flaggerv1.Canary
		expErr bool
	}{
		{newDeploymentFixture(nil), withFinalizer, false},
		{m, m.canary, true},
	}

	for _, table := range tables {
		err := table.mock.ctrl.removeFinalizer(table.canary)
		if table.expErr {
			require.NotNil(t, err)
		} else {
			require.Nil(t, err)
		}
	}
}
