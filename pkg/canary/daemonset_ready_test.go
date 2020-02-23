package canary

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDaemonSetController_IsReady(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Error("Expected primary readiness check to fail")
	}

	_, err = mocks.controller.IsPrimaryReady(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestDaemonSetController_isDaemonSetReady(t *testing.T) {
	mocks := newDaemonSetFixture()
	_, err := mocks.controller.isDaemonSetReady(&appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Generation: 10,
		},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     10,
			DesiredNumberScheduled: 1,
			UpdatedNumberScheduled: 1,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = mocks.controller.isDaemonSetReady(&appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Generation: 9,
		},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     10,
			DesiredNumberScheduled: 2,
			UpdatedNumberScheduled: 1,
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
