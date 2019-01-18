package controller

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"sync"
	"testing"
	"time"

	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	fakeIstio "github.com/knative/pkg/client/clientset/versioned/fake"
	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/stefanprodan/flagger/pkg/client/clientset/versioned/fake"
	informers "github.com/stefanprodan/flagger/pkg/client/informers/externalversions"
	"github.com/stefanprodan/flagger/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

func newTestController(
	kubeClient kubernetes.Interface,
	istioClient istioclientset.Interface,
	flaggerClient clientset.Interface,
	logger *zap.SugaredLogger,
	deployer CanaryDeployer,
	router CanaryRouter,
	observer CanaryObserver,
) *Controller {
	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, noResyncPeriodFunc())
	flaggerInformer := flaggerInformerFactory.Flagger().V1alpha3().Canaries()

	ctrl := &Controller{
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		flaggerClient: flaggerClient,
		flaggerLister: flaggerInformer.Lister(),
		flaggerSynced: flaggerInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		eventRecorder: &record.FakeRecorder{},
		logger:        logger,
		canaries:      new(sync.Map),
		flaggerWindow: time.Second,
		deployer:      deployer,
		router:        router,
		observer:      observer,
		recorder:      NewCanaryRecorder(false),
	}
	ctrl.flaggerSynced = alwaysReady

	return ctrl
}

func TestScheduler_Init(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")
	deployer := CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}
	router := CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}
	observer := CanaryObserver{
		metricsServer: "fake",
	}
	ctrl := newTestController(kubeClient, istioClient, flaggerClient, logger, deployer, router, observer)

	ctrl.advanceCanary("podinfo", "default", false)

	_, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestScheduler_NewRevision(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")
	deployer := CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}
	router := CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}
	observer := CanaryObserver{
		metricsServer: "fake",
	}
	ctrl := newTestController(kubeClient, istioClient, flaggerClient, logger, deployer, router, observer)

	// init
	ctrl.advanceCanary("podinfo", "default", false)

	// update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", false)

	c, err := kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 1 {
		t.Errorf("Got canary replicas %v wanted %v", *c.Spec.Replicas, 1)
	}
}

func TestScheduler_Rollback(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")
	deployer := CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}
	router := CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}
	observer := CanaryObserver{
		metricsServer: "fake",
	}
	ctrl := newTestController(kubeClient, istioClient, flaggerClient, logger, deployer, router, observer)

	// init
	ctrl.advanceCanary("podinfo", "default", true)

	// update failed checks to max
	err := deployer.SyncStatus(canary, v1alpha3.CanaryStatus{Phase: v1alpha3.CanaryProgressing, FailedChecks: 11})
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)

	c, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != v1alpha3.CanaryFailed {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, v1alpha3.CanaryFailed)
	}
}

func TestScheduler_NewRevisionReset(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")
	deployer := CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}
	router := CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}
	observer := CanaryObserver{
		metricsServer: "fake",
	}
	ctrl := newTestController(kubeClient, istioClient, flaggerClient, logger, deployer, router, observer)

	// init
	ctrl.advanceCanary("podinfo", "default", false)

	// first update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)
	// advance
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err := router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 90 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 90)
	}

	if canaryRoute.Weight != 10 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 10)
	}

	// second update
	dep2.Spec.Template.Spec.ServiceAccountName = "test"
	_, err = kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err = router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 100)
	}

	if canaryRoute.Weight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 0)
	}
}

func TestScheduler_Promotion(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)
	kubeClient := fake.NewSimpleClientset(dep, hpa)
	istioClient := fakeIstio.NewSimpleClientset()

	logger, _ := logging.NewLogger("debug")
	deployer := CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}
	router := CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}
	observer := CanaryObserver{
		metricsServer: "fake",
	}
	ctrl := newTestController(kubeClient, istioClient, flaggerClient, logger, deployer, router, observer)

	// init
	ctrl.advanceCanary("podinfo", "default", false)

	// update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err := router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryRoute.Weight = 60
	canaryRoute.Weight = 40
	err = ctrl.router.SetRoutes(canary, primaryRoute, canaryRoute)
	if err != nil {
		t.Fatal(err.Error())
	}

	// advance
	ctrl.advanceCanary("podinfo", "default", true)

	// promote
	ctrl.advanceCanary("podinfo", "default", true)

	primaryRoute, canaryRoute, err = router.GetRoutes(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if primaryRoute.Weight != 100 {
		t.Errorf("Got primary route %v wanted %v", primaryRoute.Weight, 100)
	}

	if canaryRoute.Weight != 0 {
		t.Errorf("Got canary route %v wanted %v", canaryRoute.Weight, 0)
	}

	primaryDep, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := primaryDep.Spec.Template.Spec.Containers[0].Image
	canaryImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != canaryImage {
		t.Errorf("Got primary image %v wanted %v", primaryImage, canaryImage)
	}

	c, err := flaggerClient.FlaggerV1alpha3().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.Status.Phase != v1alpha3.CanarySucceeded {
		t.Errorf("Got canary state %v wanted %v", c.Status.Phase, v1alpha3.CanarySucceeded)
	}
}
