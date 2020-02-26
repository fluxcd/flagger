package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestScheduler_ServicePromotion(t *testing.T) {
	mocks := newDeploymentFixture(newTestServiceCanary())

	// init
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check initialized status
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseInitialized {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseInitialized)
	}

	// update
	svc2 := newDeploymentTestServiceV2()
	_, err = mocks.kubeClient.CoreV1().Services("default").Update(svc2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect service spec changes
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryWeight = 60
	canaryWeight = 40
	err = mocks.router.SetRoutes(mocks.canary, primaryWeight, canaryWeight, mirrored)
	if err != nil {
		t.Fatal(err.Error())
	}

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check progressing status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseProgressing {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseProgressing)
	}

	// promote
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	// check promoting status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhasePromoting {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhasePromoting)
	}

	// finalise
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryWeight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryWeight, 100)
	}

	if canaryWeight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryWeight, 0)
	}

	if mirrored != false {
		t.Errorf("Got mirrored %v wanted %v", mirrored, false)
	}

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryLabelValue := primarySvc.Spec.Selector["app"]
	canaryLabelValue := svc2.Spec.Selector["app"]
	if primaryLabelValue != canaryLabelValue {
		t.Errorf("Got primary selector label value %v wanted %v", primaryLabelValue, canaryLabelValue)
	}

	// check finalising status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseFinalising {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseFinalising)
	}

	// scale canary to zero
	mocks.ctrl.advanceCanary("podinfo", "default", true)

	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != flaggerv1.CanaryPhaseSucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, flaggerv1.CanaryPhaseSucceeded)
	}
}

func newTestServiceCanary() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "core/v1",
				Kind:       "Service",
			},
			Service: flaggerv1.CanaryService{
				Port: 9898,
			},
			CanaryAnalysis: flaggerv1.CanaryAnalysis{
				Threshold:  10,
				StepWeight: 10,
				MaxWeight:  50,
				Metrics: []flaggerv1.CanaryMetric{
					{
						Name:      "request-success-rate",
						Threshold: 99,
						Interval:  "1m",
					},
					{
						Name:      "request-duration",
						Threshold: 500000,
						Interval:  "1m",
					},
				},
			},
		},
	}
	return cd
}
