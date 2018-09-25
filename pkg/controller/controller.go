package controller

import (
	"fmt"
	"time"

	"sync"

	"github.com/google/go-cmp/cmp"
	sharedclientset "github.com/knative/pkg/client/clientset/versioned"
	rolloutv1 "github.com/stefanprodan/steerer/pkg/apis/rollout/v1beta1"
	clientset "github.com/stefanprodan/steerer/pkg/client/clientset/versioned"
	rolloutscheme "github.com/stefanprodan/steerer/pkg/client/clientset/versioned/scheme"
	rolloutInformers "github.com/stefanprodan/steerer/pkg/client/informers/externalversions/rollout/v1beta1"
	listers "github.com/stefanprodan/steerer/pkg/client/listers/rollout/v1beta1"
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

const controllerAgentName = "steerer"

type Controller struct {
	kubeClient    kubernetes.Interface
	istioClient   sharedclientset.Interface
	rolloutClient clientset.Interface
	rolloutLister listers.RolloutLister
	rolloutSynced cache.InformerSynced
	rolloutWindow time.Duration
	workqueue     workqueue.RateLimitingInterface
	recorder      record.EventRecorder
	logger        *zap.SugaredLogger
	metricServer  string
	rollouts      *sync.Map
}

func NewController(
	kubeClient kubernetes.Interface,
	istioClient sharedclientset.Interface,
	rolloutClient clientset.Interface,
	rolloutInformer rolloutInformers.RolloutInformer,
	rolloutWindow time.Duration,
	metricServer string,
	logger *zap.SugaredLogger,

) *Controller {
	logger.Debug("Creating event broadcaster")
	rolloutscheme.AddToScheme(scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Named("event-broadcaster").Debugf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})
	recorder := eventBroadcaster.NewRecorder(
		scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	ctrl := &Controller{
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		rolloutClient: rolloutClient,
		rolloutLister: rolloutInformer.Lister(),
		rolloutSynced: rolloutInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		recorder:      recorder,
		logger:        logger,
		rollouts:      new(sync.Map),
		metricServer:  metricServer,
		rolloutWindow: rolloutWindow,
	}

	rolloutInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.enqueueRollout,
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
				ctrl.enqueueRollout(new)
			}
		},
		DeleteFunc: func(old interface{}) {
			r, ok := checkCustomResourceType(old, logger)
			if ok {
				ctrl.logger.Infof("Deleting %s.%s from cache", r.Name, r.Namespace)
				ctrl.rollouts.Delete(fmt.Sprintf("%s.%s", r.Name, r.Namespace))
			}
		},
	})

	return ctrl
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	c.logger.Info("Starting controller")

	for i := 0; i < threadiness; i++ {
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stopCh)
	}

	c.logger.Info("Started workers")

	tickChan := time.NewTicker(c.rolloutWindow).C
	for {
		select {
		case <-tickChan:
			c.doRollouts()
		case <-stopCh:
			c.logger.Info("Shutting down workers")
			return nil
		}
	}

	return nil
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
		c.logger.Infof("Successfully synced '%s'", key)
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

	rollout, err := c.rolloutLister.Rollouts(namespace).Get(name)
	if errors.IsNotFound(err) {
		utilruntime.HandleError(fmt.Errorf("rollout '%s' in work queue no longer exists", key))
		return nil
	}

	c.logger.Infof("Adding %s.%s to cache", rollout.Name, rollout.Namespace)
	c.rollouts.Store(fmt.Sprintf("%s.%s", rollout.Name, rollout.Namespace), rollout)
	//c.recorder.Event(rollout, corev1.EventTypeNormal, "Synced", "Rollout synced successfully with internal cache")

	return nil
}

func (c *Controller) enqueueRollout(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		c.logger.Debugf("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	c.logger.Debugf("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		if ownerRef.Kind != "Rollout" {
			return
		}

		vs, err := c.rolloutLister.Rollouts(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			c.logger.Debugf("ignoring orphaned object '%s' of '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueRollout(vs)
		return
	}

}

func (c *Controller) recordEventInfof(r *rolloutv1.Rollout, template string, args ...interface{}) {
	c.logger.Infof(template, args...)
	c.recorder.Event(r, corev1.EventTypeNormal, "Synced", fmt.Sprintf(template, args...))
}

func (c *Controller) recordEventErrorf(r *rolloutv1.Rollout, template string, args ...interface{}) {
	c.logger.Errorf(template, args...)
	c.recorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
}

func checkCustomResourceType(obj interface{}, logger *zap.SugaredLogger) (rolloutv1.Rollout, bool) {
	var roll *rolloutv1.Rollout
	var ok bool
	if roll, ok = obj.(*rolloutv1.Rollout); !ok {
		logger.Errorf("Event Watch received an invalid object: %#v", obj)
		return rolloutv1.Rollout{}, false
	}
	return *roll, true
}
