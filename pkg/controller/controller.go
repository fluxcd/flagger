package controller

import (
	"fmt"
	"sync"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	"github.com/weaveworks/flagger/pkg/canary"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	flaggerscheme "github.com/weaveworks/flagger/pkg/client/clientset/versioned/scheme"
	flaggerinformers "github.com/weaveworks/flagger/pkg/client/informers/externalversions/flagger/v1alpha3"
	flaggerlisters "github.com/weaveworks/flagger/pkg/client/listers/flagger/v1alpha3"
	"github.com/weaveworks/flagger/pkg/metrics"
	"github.com/weaveworks/flagger/pkg/notifier"
	"github.com/weaveworks/flagger/pkg/router"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const controllerAgentName = "flagger"

// Controller is managing the canary objects and schedules canary deployments
type Controller struct {
	kubeClient      kubernetes.Interface
	istioClient     clientset.Interface
	flaggerClient   clientset.Interface
	flaggerLister   flaggerlisters.CanaryLister
	flaggerSynced   cache.InformerSynced
	flaggerWindow   time.Duration
	workqueue       workqueue.RateLimitingInterface
	eventRecorder   record.EventRecorder
	logger          *zap.SugaredLogger
	canaries        *sync.Map
	jobs            map[string]CanaryJob
	deployer        canary.Deployer
	recorder        metrics.Recorder
	notifier        notifier.Interface
	routerFactory   *router.Factory
	observerFactory *metrics.Factory
	meshProvider    string
}

func NewController(
	kubeClient kubernetes.Interface,
	istioClient clientset.Interface,
	flaggerClient clientset.Interface,
	flaggerInformer flaggerinformers.CanaryInformer,
	flaggerWindow time.Duration,
	logger *zap.SugaredLogger,
	notifier notifier.Interface,
	routerFactory *router.Factory,
	observerFactory *metrics.Factory,
	meshProvider string,
	version string,
	labels []string,
) *Controller {
	logger.Debug("Creating event broadcaster")
	flaggerscheme.AddToScheme(scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Named("event-broadcaster").Debugf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})
	eventRecorder := eventBroadcaster.NewRecorder(
		scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	deployer := canary.Deployer{
		Logger:        logger,
		KubeClient:    kubeClient,
		FlaggerClient: flaggerClient,
		Labels:        labels,
		ConfigTracker: canary.ConfigTracker{
			Logger:        logger,
			KubeClient:    kubeClient,
			FlaggerClient: flaggerClient,
		},
	}

	recorder := metrics.NewRecorder(controllerAgentName, true)
	recorder.SetInfo(version, meshProvider)

	ctrl := &Controller{
		kubeClient:      kubeClient,
		istioClient:     istioClient,
		flaggerClient:   flaggerClient,
		flaggerLister:   flaggerInformer.Lister(),
		flaggerSynced:   flaggerInformer.Informer().HasSynced,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		eventRecorder:   eventRecorder,
		logger:          logger,
		canaries:        new(sync.Map),
		jobs:            map[string]CanaryJob{},
		flaggerWindow:   flaggerWindow,
		deployer:        deployer,
		observerFactory: observerFactory,
		recorder:        recorder,
		notifier:        notifier,
		routerFactory:   routerFactory,
		meshProvider:    meshProvider,
	}

	flaggerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.enqueue,
		UpdateFunc: func(old, new interface{}) {
			oldRoll, ok := checkCustomResourceType(old, logger)
			if !ok {
				return
			}
			newRoll, ok := checkCustomResourceType(new, logger)
			if !ok {
				return
			}

			if diff := cmp.Diff(newRoll.Spec, oldRoll.Spec); diff != "" {
				ctrl.logger.Debugf("Diff detected %s.%s %s", oldRoll.Name, oldRoll.Namespace, diff)
				ctrl.enqueue(new)
			}
		},
		DeleteFunc: func(old interface{}) {
			r, ok := checkCustomResourceType(old, logger)
			if ok {
				ctrl.logger.Infof("Deleting %s.%s from cache", r.Name, r.Namespace)
				ctrl.canaries.Delete(fmt.Sprintf("%s.%s", r.Name, r.Namespace))
			}
		},
	})

	return ctrl
}

// Run starts the K8s workers and the canary scheduler
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	c.logger.Info("Starting operator")

	for i := 0; i < threadiness; i++ {
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stopCh)
	}

	c.logger.Info("Started operator workers")

	tickChan := time.NewTicker(c.flaggerWindow).C
	for {
		select {
		case <-tickChan:
			c.scheduleCanaries()
		case <-stopCh:
			c.logger.Info("Shutting down operator workers")
			return nil
		}
	}
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	cd, err := c.flaggerLister.Canaries(namespace).Get(name)
	if errors.IsNotFound(err) {
		utilruntime.HandleError(fmt.Errorf("%s in work queue no longer exists", key))
		return nil
	}

	// set status condition for new canaries
	if cd.Status.Conditions == nil {
		if ok, conditions := c.deployer.MakeStatusConditions(cd.Status, flaggerv1.CanaryPhaseInitializing); ok {
			cdCopy := cd.DeepCopy()
			cdCopy.Status.Conditions = conditions
			cdCopy.Status.LastTransitionTime = metav1.Now()
			cdCopy.Status.Phase = flaggerv1.CanaryPhaseInitializing
			_, err := c.flaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).UpdateStatus(cdCopy)
			if err != nil {
				c.logger.Errorf("%s status condition update error: %v", key, err)
				return fmt.Errorf("%s status condition update error: %v", key, err)
			}
		}
	}

	c.canaries.Store(fmt.Sprintf("%s.%s", cd.Name, cd.Namespace), cd)
	c.logger.Infof("Synced %s", key)

	return nil
}

func (c *Controller) enqueue(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func checkCustomResourceType(obj interface{}, logger *zap.SugaredLogger) (flaggerv1.Canary, bool) {
	var roll *flaggerv1.Canary
	var ok bool
	if roll, ok = obj.(*flaggerv1.Canary); !ok {
		logger.Errorf("Event Watch received an invalid object: %#v", obj)
		return flaggerv1.Canary{}, false
	}
	return *roll, true
}

func (c *Controller) recordEventInfof(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Infof(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeNormal, "Synced", fmt.Sprintf(template, args...))
}

func (c *Controller) recordEventErrorf(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Errorf(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
}

func (c *Controller) recordEventWarningf(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Infof(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
}

func (c *Controller) sendNotification(cd *flaggerv1.Canary, message string, metadata bool, warn bool) {
	if c.notifier == nil {
		return
	}

	var fields []notifier.Field

	if metadata {
		fields = append(fields,
			notifier.Field{
				Name:  "Target",
				Value: fmt.Sprintf("%s/%s.%s", cd.Spec.TargetRef.Kind, cd.Spec.TargetRef.Name, cd.Namespace),
			},
			notifier.Field{
				Name:  "Failed checks threshold",
				Value: fmt.Sprintf("%v", cd.Spec.CanaryAnalysis.Threshold),
			},
			notifier.Field{
				Name:  "Progress deadline",
				Value: fmt.Sprintf("%vs", cd.GetProgressDeadlineSeconds()),
			},
		)

		if cd.Spec.CanaryAnalysis.StepWeight > 0 {
			fields = append(fields, notifier.Field{
				Name: "Traffic routing",
				Value: fmt.Sprintf("Weight step: %v max: %v",
					cd.Spec.CanaryAnalysis.StepWeight,
					cd.Spec.CanaryAnalysis.MaxWeight),
			})
		} else if len(cd.Spec.CanaryAnalysis.Match) > 0 {
			fields = append(fields, notifier.Field{
				Name:  "Traffic routing",
				Value: "A/B Testing",
			})
		} else if cd.Spec.CanaryAnalysis.Iterations > 0 {
			fields = append(fields, notifier.Field{
				Name:  "Traffic routing",
				Value: "Blue/Green",
			})
		}
	}
	err := c.notifier.Post(cd.Name, cd.Namespace, message, fields, warn)
	if err != nil {
		c.logger.Error(err)
	}
}

func int32p(i int32) *int32 {
	return &i
}
