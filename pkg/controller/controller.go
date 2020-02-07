package controller

import (
	"fmt"
	"sync"
	"time"

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

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	flaggerscheme "github.com/weaveworks/flagger/pkg/client/clientset/versioned/scheme"
	flaggerinformers "github.com/weaveworks/flagger/pkg/client/informers/externalversions/flagger/v1beta1"
	flaggerlisters "github.com/weaveworks/flagger/pkg/client/listers/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics"
	"github.com/weaveworks/flagger/pkg/metrics/observers"
	"github.com/weaveworks/flagger/pkg/notifier"
	"github.com/weaveworks/flagger/pkg/router"
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
	recorder        metrics.Recorder
	notifier        notifier.Interface
	canaryFactory   *canary.Factory
	routerFactory   *router.Factory
	observerFactory *observers.Factory
	meshProvider    string
	eventWebhook    string
}

func NewController(
	kubeClient kubernetes.Interface,
	istioClient clientset.Interface,
	flaggerClient clientset.Interface,
	flaggerInformer flaggerinformers.CanaryInformer,
	flaggerWindow time.Duration,
	logger *zap.SugaredLogger,
	notifier notifier.Interface,
	canaryFactory *canary.Factory,
	routerFactory *router.Factory,
	observerFactory *observers.Factory,
	meshProvider string,
	version string,
	eventWebhook string,
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
		observerFactory: observerFactory,
		recorder:        recorder,
		notifier:        notifier,
		canaryFactory:   canaryFactory,
		routerFactory:   routerFactory,
		meshProvider:    meshProvider,
		eventWebhook:    eventWebhook,
	}

	flaggerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.enqueue,
		UpdateFunc: func(old, new interface{}) {
			oldCanary, ok := checkCustomResourceType(old, logger)
			if !ok {
				return
			}
			newCanary, ok := checkCustomResourceType(new, logger)
			if !ok {
				return
			}

			if diff := cmp.Diff(newCanary.Spec, oldCanary.Spec); diff != "" {
				ctrl.logger.Debugf("Diff detected %s.%s %s", oldCanary.Name, oldCanary.Namespace, diff)

				// warn about routing conflicts when service name changes
				if oldCanary.Spec.Service.Name != "" && oldCanary.Spec.Service.Name != newCanary.Spec.Service.Name {
					ctrl.logger.With("canary", fmt.Sprintf("%s.%s", oldCanary.Name, oldCanary.Namespace)).
						Warnf("The service name changed to %s, remove %s objects to avoid routing conflicts",
							newCanary.Spec.Service.Name, oldCanary.Spec.Service.Name)
				}

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
		if ok, conditions := canary.MakeStatusConditions(cd.Status, flaggerv1.CanaryPhaseInitializing); ok {
			cdCopy := cd.DeepCopy()
			cdCopy.Status.Conditions = conditions
			cdCopy.Status.LastTransitionTime = metav1.Now()
			cdCopy.Status.Phase = flaggerv1.CanaryPhaseInitializing
			_, err := c.flaggerClient.FlaggerV1beta1().Canaries(cd.Namespace).UpdateStatus(cdCopy)
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

func (c *Controller) sendEventToWebhook(r *flaggerv1.Canary, eventtype, template string, args []interface{}) {
	webhookOverride := false
	if len(r.Spec.CanaryAnalysis.Webhooks) > 0 {
		for _, canaryWebhook := range r.Spec.CanaryAnalysis.Webhooks {
			if canaryWebhook.Type == flaggerv1.EventHook {
				webhookOverride = true
				err := CallEventWebhook(r, canaryWebhook.URL, fmt.Sprintf(template, args...), eventtype)
				if err != nil {
					c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Errorf("error sending event to webhook: %s", err)
				}
			}
		}
	}

	if c.eventWebhook != "" && !webhookOverride {
		err := CallEventWebhook(r, c.eventWebhook, fmt.Sprintf(template, args...), eventtype)
		if err != nil {
			c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Errorf("error sending event to webhook: %s", err)
		}
	}
}

func (c *Controller) recordEventInfof(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Infof(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeNormal, "Synced", fmt.Sprintf(template, args...))
	c.sendEventToWebhook(r, corev1.EventTypeNormal, template, args)
}

func (c *Controller) recordEventErrorf(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Errorf(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
	c.sendEventToWebhook(r, corev1.EventTypeWarning, template, args)
}

func (c *Controller) recordEventWarningf(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Infof(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
	c.sendEventToWebhook(r, corev1.EventTypeWarning, template, args)
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
