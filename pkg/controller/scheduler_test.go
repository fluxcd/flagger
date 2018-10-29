package controller

import (
	"sync"
	"testing"
	"time"

	fakeIstio "github.com/knative/pkg/client/clientset/versioned/fake"
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

	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, noResyncPeriodFunc())
	flaggerInformer := flaggerInformerFactory.Flagger().V1alpha1().Canaries()

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
	}
	ctrl.flaggerSynced = alwaysReady

	ctrl.advanceCanary("podinfo", "default")

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

	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, noResyncPeriodFunc())
	flaggerInformer := flaggerInformerFactory.Flagger().V1alpha1().Canaries()

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
	}
	ctrl.flaggerSynced = alwaysReady

	// init
	ctrl.advanceCanary("podinfo", "default")

	// update
	dep2 := newTestDeploymentUpdated()
	_, err := kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// detect changes
	ctrl.advanceCanary("podinfo", "default")

	c, err := kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 1 {
		t.Errorf("Got canary replicas %v wanted %v", *c.Spec.Replicas, 1)
	}
}
