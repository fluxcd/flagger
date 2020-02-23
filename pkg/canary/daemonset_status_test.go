package canary

import (
	"testing"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDaemonSetController_SyncStatus(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	status := flaggerv1.CanaryStatus{
		Phase:        flaggerv1.CanaryPhaseProgressing,
		FailedChecks: 2,
	}
	err = mocks.controller.SyncStatus(mocks.canary, status)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != status.Phase {
		t.Errorf("Got state %v wanted %v", res.Status.Phase, status.Phase)
	}

	if res.Status.FailedChecks != status.FailedChecks {
		t.Errorf("Got failed checks %v wanted %v", res.Status.FailedChecks, status.FailedChecks)
	}

	if res.Status.TrackedConfigs == nil {
		t.Fatalf("Status tracking configs are empty")
	}
	configs := *res.Status.TrackedConfigs
	secret := newDaemonSetControllerTestSecret()
	if _, exists := configs["secret/"+secret.GetName()]; !exists {
		t.Errorf("Secret %s not found in status", secret.GetName())
	}
}

func TestDaemonSetController_SetFailedChecks(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.controller.SetStatusFailedChecks(mocks.canary, 1)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.FailedChecks != 1 {
		t.Errorf("Got %v wanted %v", res.Status.FailedChecks, 1)
	}
}

func TestDaemonSetController_SetState(t *testing.T) {
	mocks := newDaemonSetFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = mocks.controller.SetStatusPhase(mocks.canary, flaggerv1.CanaryPhaseProgressing)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.Phase != flaggerv1.CanaryPhaseProgressing {
		t.Errorf("Got %v wanted %v", res.Status.Phase, flaggerv1.CanaryPhaseProgressing)
	}
}
