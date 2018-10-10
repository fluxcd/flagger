package controller

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	flaggerscheme "github.com/stefanprodan/flagger/pkg/client/clientset/versioned/scheme"
	flaggerinformers "github.com/stefanprodan/flagger/pkg/client/informers/externalversions/flagger/v1alpha1"
	flaggerlisters "github.com/stefanprodan/flagger/pkg/client/listers/flagger/v1alpha1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

type Controller struct {
	kubeClient    kubernetes.Interface
	istioClient   istioclientset.Interface
	rolloutClient clientset.Interface
	rolloutLister flaggerlisters.CanaryLister
	rolloutSynced cache.InformerSynced
	rolloutWindow time.Duration
	workqueue     workqueue.RateLimitingInterface
	recorder      record.EventRecorder
	logger        *zap.SugaredLogger
	metricsServer string
	rollouts      *sync.Map
}

func NewController(
	kubeClient kubernetes.Interface,
	istioClient istioclientset.Interface,
	rolloutClient clientset.Interface,
	rolloutInformer flaggerinformers.CanaryInformer,
	rolloutWindow time.Duration,
	metricServer string,
	logger *zap.SugaredLogger,

) *Controller {
	logger.Debug("Creating event broadcaster")
	flaggerscheme.AddToScheme(scheme.Scheme)
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
		metricsServer: metricServer,
		rolloutWindow: rolloutWindow,
	}

	rolloutInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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
				ctrl.rollouts.Delete(fmt.Sprintf("%s.%s", r.Name, r.Namespace))
			}
		},
	})

	return ctrl
}

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

	tickChan := time.NewTicker(c.rolloutWindow).C
	for {
		select {
		case <-tickChan:
			c.doRollouts()
		case <-stopCh:
			c.logger.Info("Shutting down operator workers")
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
	cd, err := c.rolloutLister.Canaries(namespace).Get(name)
	if errors.IsNotFound(err) {
		utilruntime.HandleError(fmt.Errorf("'%s' in work queue no longer exists", key))
		return nil
	}

	c.rollouts.Store(fmt.Sprintf("%s.%s", cd.Name, cd.Namespace), cd)

	err = c.bootstrapDeployment(cd)
	if err != nil {
		c.logger.Warnf("%s.%s bootstrap error %v", cd.Name, cd.Namespace, err)
		return err
	}

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
	c.logger.Infof(template, args...)
	c.recorder.Event(r, corev1.EventTypeNormal, "Synced", fmt.Sprintf(template, args...))
}

func (c *Controller) recordEventErrorf(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.Errorf(template, args...)
	c.recorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
}

func (c *Controller) recordEventWarningf(r *flaggerv1.Canary, template string, args ...interface{}) {
	c.logger.Infof(template, args...)
	c.recorder.Event(r, corev1.EventTypeWarning, "Synced", fmt.Sprintf(template, args...))
}
