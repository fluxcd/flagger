package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/notifier"
)

func TestScheduler_DeploymentInit(t *testing.T) {
	mocks := newDeploymentFixture(nil)
	mocks.ctrl.advanceCanary("podinfo", "default")

	_, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestScheduler_DeploymentNewRevision(t *testing.T) {
	mocks := newDeploymentFixture(nil)

	// initializing ...
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialization done
	mocks.ctrl.advanceCanary("podinfo", "default")

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default")

	c, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, int32(1), *c.Spec.Replicas)
}

func TestScheduler_DeploymentRollback(t *testing.T) {
	mocks := newDeploymentFixture(nil)
	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")

	// update failed checks to max
	err := mocks.deployer.SyncStatus(mocks.canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing, FailedChecks: 10})
	require.NoError(t, err)

	// set a metric check to fail
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	cd := c.DeepCopy()
	cd.Spec.Analysis.Metrics = append(c.Spec.Analysis.Metrics, flaggerv1.CanaryMetric{
		Name:     "fail",
		Interval: "1m",
		ThresholdRange: &flaggerv1.CanaryThresholdRange{
			Min: toFloatPtr(0),
			Max: toFloatPtr(50),
		},
		Query: "fail",
	})
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
	require.NoError(t, err)

	// run metric checks
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, err)

	// finalise analysis
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, err)

	// check status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, flaggerv1.CanaryPhaseFailed, c.Status.Phase)
}

func TestScheduler_DeploymentSkipAnalysis(t *testing.T) {
	mocks := newDeploymentFixture(nil)
	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")

	// enable skip
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	cd.Spec.SkipAnalysis = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
	require.NoError(t, err)

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default")
	mocks.makeCanaryReady(t)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, c.Spec.SkipAnalysis)
	assert.Equal(t, flaggerv1.CanaryPhaseSucceeded, c.Status.Phase)
}

func TestScheduler_DeploymentAnalysisPhases(t *testing.T) {
	cd := newDeploymentTestCanary()
	cd.Spec.Analysis = &flaggerv1.CanaryAnalysis{
		Interval:   "1m",
		StepWeight: 100,
	}
	mocks := newDeploymentFixture(cd)

	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseInitialized))

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))
	mocks.makeCanaryReady(t)

	// progressing
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))

	// promoting
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhasePromoting))

	// finalising
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseFinalising))

	// succeeded
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseSucceeded))
}

func TestScheduler_DeploymentBlueGreenAnalysisPhases(t *testing.T) {
	cd := newDeploymentTestCanary()
	cd.Spec.Analysis = &flaggerv1.CanaryAnalysis{
		Interval:   "1m",
		Iterations: 1,
	}
	mocks := newDeploymentFixture(cd)

	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseInitialized))

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect changes (progressing)
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))
	mocks.makeCanaryReady(t)

	// advance (progressing)
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))

	// route traffic to primary (progressing)
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseProgressing))

	// promoting
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhasePromoting))

	// finalising
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseFinalising))

	// succeeded
	mocks.ctrl.advanceCanary("podinfo", "default")
	require.NoError(t, assertPhase(mocks.flaggerClient, "podinfo", flaggerv1.CanaryPhaseSucceeded))
}

func TestScheduler_DeploymentNewRevisionReset(t *testing.T) {
	mocks := newDeploymentFixture(nil)
	// init
	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")

	// first update
	dep2 := newDeploymentTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default")
	mocks.makeCanaryReady(t)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 90, primaryWeight)
	assert.Equal(t, 10, canaryWeight)
	assert.False(t, mirrored)

	// second update
	dep2.Spec.Template.Spec.ServiceAccountName = "test"
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect changes
	mocks.ctrl.advanceCanary("podinfo", "default")

	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, primaryWeight)
	assert.Equal(t, 0, canaryWeight)
	assert.False(t, mirrored)
}

func TestScheduler_DeploymentPromotion(t *testing.T) {
	mocks := newDeploymentFixture(nil)

	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check initialized status
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseInitialized, c.Status.Phase)

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err = mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect pod spec changes
	mocks.ctrl.advanceCanary("podinfo", "default")
	mocks.makeCanaryReady(t)

	config2 := newDeploymentTestConfigMapV2()
	_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Update(context.TODO(), config2, metav1.UpdateOptions{})
	require.NoError(t, err)

	secret2 := newDeploymentTestSecretV2()
	_, err = mocks.kubeClient.CoreV1().Secrets("default").Update(context.TODO(), secret2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect configs changes
	mocks.ctrl.advanceCanary("podinfo", "default")

	_, _, _, err = mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)

	primaryWeight := 60
	canaryWeight := 40
	err = mocks.router.SetRoutes(mocks.canary, primaryWeight, canaryWeight, false)
	require.NoError(t, err)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check progressing status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseProgressing, c.Status.Phase)

	// promote
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check promoting status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhasePromoting, c.Status.Phase)

	// finalise
	mocks.ctrl.advanceCanary("podinfo", "default")

	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, primaryWeight)
	assert.Equal(t, 0, canaryWeight)
	assert.False(t, mirrored)

	primaryDep, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	primaryImage := primaryDep.Spec.Template.Spec.Containers[0].Image
	canaryImage := dep2.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, canaryImage, primaryImage)

	configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, config2.Data["color"], configPrimary.Data["color"])

	secretPrimary, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-env-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, string(secret2.Data["apiKey"]), string(secretPrimary.Data["apiKey"]))

	// check finalising status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseFinalising, c.Status.Phase)

	// scale canary to zero
	mocks.ctrl.advanceCanary("podinfo", "default")

	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseSucceeded, c.Status.Phase)
}

func TestScheduler_DeploymentMirroring(t *testing.T) {
	mocks := newDeploymentFixture(newDeploymentTestCanaryMirror())

	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect pod spec changes
	mocks.ctrl.advanceCanary("podinfo", "default")
	mocks.makeCanaryReady(t)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check if traffic is mirrored to canary
	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 100, primaryWeight)
	assert.Equal(t, 0, canaryWeight)
	assert.True(t, mirrored)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check if traffic is mirrored to canary
	primaryWeight, canaryWeight, mirrored, err = mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 90, primaryWeight)
	assert.Equal(t, 10, canaryWeight)
	assert.False(t, mirrored)
}

func TestScheduler_DeploymentABTesting(t *testing.T) {
	mocks := newDeploymentFixture(newDeploymentTestCanaryAB())
	// initializing
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialized
	mocks.ctrl.advanceCanary("podinfo", "default")

	// update
	dep2 := newDeploymentTestDeploymentV2()
	_, err := mocks.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), dep2, metav1.UpdateOptions{})
	require.NoError(t, err)

	// detect pod spec changes
	mocks.ctrl.advanceCanary("podinfo", "default")
	mocks.makeCanaryReady(t)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check if traffic is routed to canary
	primaryWeight, canaryWeight, mirrored, err := mocks.router.GetRoutes(mocks.canary)
	require.NoError(t, err)
	assert.Equal(t, 0, primaryWeight)
	assert.Equal(t, 100, canaryWeight)
	assert.False(t, mirrored)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// set max iterations
	err = mocks.deployer.SetStatusIterations(cd, 10)
	require.NoError(t, err)

	// advance
	mocks.ctrl.advanceCanary("podinfo", "default")

	// finalising
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check finalising status
	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseFinalising, c.Status.Phase)

	// check if the container image tag was updated
	primaryDep, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	primaryImage := primaryDep.Spec.Template.Spec.Containers[0].Image
	canaryImage := dep2.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, canaryImage, primaryImage)

	// shutdown canary
	mocks.ctrl.advanceCanary("podinfo", "default")

	// check rollout status
	c, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, flaggerv1.CanaryPhaseSucceeded, c.Status.Phase)
}

func TestScheduler_DeploymentPortDiscovery(t *testing.T) {
	mocks := newDeploymentFixture(nil)

	// enable port discovery
	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	cd.Spec.Service.PortDiscovery = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
	require.NoError(t, err)

	mocks.ctrl.advanceCanary("podinfo", "default")

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, canarySvc.Spec.Ports, 3)

	matchPorts := func(lookup string) bool {
		switch lookup {
		case
			"http 9898",
			"http-metrics 8080",
			"tcp-podinfo-2 8888":
			return true
		}
		return false
	}

	for _, port := range canarySvc.Spec.Ports {
		require.True(t, matchPorts(fmt.Sprintf("%s %v", port.Name, port.Port)))
	}
}

func TestScheduler_DeploymentTargetPortNumber(t *testing.T) {
	mocks := newDeploymentFixture(nil)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	cd.Spec.Service.Port = 80
	cd.Spec.Service.TargetPort = intstr.FromInt(9898)
	cd.Spec.Service.PortDiscovery = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
	require.NoError(t, err)

	mocks.ctrl.advanceCanary("podinfo", "default")

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, canarySvc.Spec.Ports, 3)

	matchPorts := func(lookup string) bool {
		switch lookup {
		case
			"http 80",
			"http-metrics 8080",
			"tcp-podinfo-2 8888":
			return true
		}
		return false
	}

	for _, port := range canarySvc.Spec.Ports {
		require.True(t, matchPorts(fmt.Sprintf("%s %v", port.Name, port.Port)))
	}
}

func TestScheduler_DeploymentTargetPortName(t *testing.T) {
	mocks := newDeploymentFixture(nil)

	cd, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	cd.Spec.Service.Port = 8080
	cd.Spec.Service.TargetPort = intstr.FromString("http")
	cd.Spec.Service.PortDiscovery = true
	_, err = mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), cd, metav1.UpdateOptions{})
	require.NoError(t, err)

	mocks.ctrl.advanceCanary("podinfo", "default")

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, canarySvc.Spec.Ports, 3)

	matchPorts := func(lookup string) bool {
		switch lookup {
		case
			"http 8080",
			"http-metrics 8080",
			"tcp-podinfo-2 8888":
			return true
		}
		return false
	}

	for _, port := range canarySvc.Spec.Ports {
		require.True(t, matchPorts(fmt.Sprintf("%s %v", port.Name, port.Port)))
	}
}

func TestScheduler_DeploymentAlerts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)
		var payload = notifier.SlackPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)
		require.Equal(t, "podinfo.default", payload.Attachments[0].AuthorName)
	}))
	defer ts.Close()

	canary := newDeploymentTestCanary()
	canary.Spec.Analysis.Alerts = []flaggerv1.CanaryAlert{
		{
			Name:     "slack-dev",
			Severity: "info",
			ProviderRef: flaggerv1.CrossNamespaceObjectReference{
				Name:      "slack",
				Namespace: "default",
			},
		},
		{
			Name:     "slack-prod",
			Severity: "info",
			ProviderRef: flaggerv1.CrossNamespaceObjectReference{
				Name: "slack",
			},
		},
	}
	mocks := newDeploymentFixture(canary)

	secret := newDeploymentTestAlertProviderSecret()
	secret.Data = map[string][]byte{
		"address": []byte(ts.URL),
	}
	_, err := mocks.kubeClient.CoreV1().Secrets("default").Update(context.TODO(), secret, metav1.UpdateOptions{})
	require.NoError(t, err)

	// init canary
	mocks.ctrl.advanceCanary("podinfo", "default")

	// make primary ready
	mocks.makePrimaryReady(t)

	// initialization done - now send alert
	mocks.ctrl.advanceCanary("podinfo", "default")
}
