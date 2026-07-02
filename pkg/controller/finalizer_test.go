/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTesting "k8s.io/client-go/testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
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

func TestFinalizer_finalizeReturnsReadinessError(t *testing.T) {
	mocks := newDeploymentFixture(nil)
	mocks.canary.Spec.Provider = flaggerv1.KubernetesProvider

	dep, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	dep.Status = appsv1.DeploymentStatus{
		ObservedGeneration: dep.Generation,
		Conditions: []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentProgressing,
				Status: "False",
				Reason: "ProgressDeadlineExceeded",
			},
		},
	}

	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = mocks.ctrl.finalize(mocks.canary)
	require.Error(t, err)
	require.ErrorContains(t, err, "canary not ready during finalizing")
}
