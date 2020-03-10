package controller

import (
	"fmt"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTesting "k8s.io/client-go/testing"

	"testing"
)

//Test has finalizers
func TestFinalizer_hasFinalizer(t *testing.T) {

	withFinalizer := newDeploymentTestCanary()
	withFinalizer.Finalizers = append(withFinalizer.Finalizers, finalizer)

	tables := []struct {
		canary *flaggerv1.Canary
		result bool
	}{
		{newDeploymentTestCanary(), false},
		{withFinalizer, true},
	}

	for _, table := range tables {
		isPresent := hasFinalizer(table.canary, finalizer)
		if isPresent != table.result {
			t.Errorf("Result of hasFinalizer returned [%t], but expected [%t]", isPresent, table.result)
		}
	}
}

func TestFinalizer_addFinalizer(t *testing.T) {

	mockError := fmt.Errorf("failed to add finalizer to canary %s", "testCanary")
	cs := fakeFlagger.NewSimpleClientset(newDeploymentTestCanary())
	//prepend so it is evaluated over the catch all *
	cs.PrependReactor("update", "canaries", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, mockError
	})
	m := fixture{
		canary:        newDeploymentTestCanary(),
		flaggerClient: cs,
		ctrl:          &Controller{flaggerClient: cs},
	}

	tables := []struct {
		mock   fixture
		canary *flaggerv1.Canary
		error  error
	}{
		{newDeploymentFixture(nil), newDeploymentTestCanary(), nil},
		{m, m.canary, mockError},
	}

	for _, table := range tables {
		response := table.mock.ctrl.addFinalizer(table.canary, finalizer)

		if table.error != nil && response == nil {
			t.Errorf("Expected an error from addFinalizer, but wasn't present")
		} else if table.error == nil && response != nil {
			t.Errorf("Expected no error from addFinalizer, but returned error %s", response)
		}
	}

}

func TestFinalizer_removeFinalizer(t *testing.T) {

	withFinalizer := newDeploymentTestCanary()
	withFinalizer.Finalizers = append(withFinalizer.Finalizers, finalizer)

	mockError := fmt.Errorf("failed to add finalizer to canary %s", "testCanary")
	cs := fakeFlagger.NewSimpleClientset(newDeploymentTestCanary())
	//prepend so it is evaluated over the catch all *
	cs.PrependReactor("update", "canaries", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, mockError
	})
	m := fixture{
		canary:        withFinalizer,
		flaggerClient: cs,
		ctrl:          &Controller{flaggerClient: cs},
	}

	tables := []struct {
		mock   fixture
		canary *flaggerv1.Canary
		error  error
	}{
		{newDeploymentFixture(nil), withFinalizer, nil},
		{m, m.canary, mockError},
	}

	for _, table := range tables {
		response := table.mock.ctrl.removeFinalizer(table.canary, finalizer)

		if table.error != nil && response == nil {
			t.Errorf("Expected an error from addFinalizer, but wasn't present")
		} else if table.error == nil && response != nil {
			t.Errorf("Expected no error from addFinalizer, but returned error %s", response)
		}

	}
}
