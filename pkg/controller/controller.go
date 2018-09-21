package controller

import (
	"fmt"
	"time"

	"github.com/knative/pkg/apis/istio/v1alpha3"
	sharedclientset "github.com/knative/pkg/client/clientset/versioned"
	istioinformers "github.com/knative/pkg/client/informers/externalversions/istio/v1alpha3"
	istiolisters "github.com/knative/pkg/client/listers/istio/v1alpha3"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const controllerAgentName = "steerer"

type Controller struct {
	kubeclientset        kubernetes.Interface
	sharedclientset      sharedclientset.Interface
	logger               *zap.SugaredLogger
	serviceLister        corev1listers.ServiceLister
	serviceSynced        cache.InformerSynced
	virtualServiceLister istiolisters.VirtualServiceLister
	virtualServiceSynced cache.InformerSynced
	workqueue            workqueue.RateLimitingInterface
	recorder             record.EventRecorder
}

func NewController(
	kubeclientset kubernetes.Interface,
	sharedclientset sharedclientset.Interface,
	logger *zap.SugaredLogger,
	serviceInformer corev1informers.ServiceInformer,
	virtualServiceInformer istioinformers.VirtualServiceInformer,
) *Controller {
	logger.Debug("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Named("event-broadcaster").Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: kubeclientset.CoreV1().Events(""),
	})
	recorder := eventBroadcaster.NewRecorder(
		scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	ctrl := &Controller{
		kubeclientset:        kubeclientset,
		sharedclientset:      sharedclientset,
		logger:               logger,
		serviceLister:        serviceInformer.Lister(),
		serviceSynced:        serviceInformer.Informer().HasSynced,
		virtualServiceLister: virtualServiceInformer.Lister(),
		virtualServiceSynced: virtualServiceInformer.Informer().HasSynced,
		workqueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		recorder:             recorder,
	}

	virtualServiceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.enqueueVirtualService,
		UpdateFunc: func(old, new interface{}) {
			ctrl.enqueueVirtualService(new)
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
	<-stopCh
	c.logger.Info("Shutting down workers")

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

	vs, err := c.virtualServiceLister.VirtualServices(namespace).Get(name)
	if errors.IsNotFound(err) {
		utilruntime.HandleError(fmt.Errorf("VirtualServices '%s' in work queue no longer exists", key))
		return nil
	}

	c.logger.Infof("VirtualService %s.%s", vs.Name, namespace)

	return nil
}

func (c *Controller) enqueueVirtualService(obj interface{}) {
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
		if ownerRef.Kind != "VirtualService" {
			return
		}

		vs, err := c.serviceLister.Services(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			c.logger.Debugf("ignoring orphaned object '%s' of '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueVirtualService(vs)
		return
	}

}

func (c *Controller) CreateVirtualService(namespace string, name string, host string, port uint32, gateway string) error {
	vs := &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1alpha3.VirtualServiceSpec{
			Hosts: []string{host},
			Http: []v1alpha3.HTTPRoute{
				{
					Route: []v1alpha3.DestinationWeight{
						{
							Destination: v1alpha3.Destination{
								Host: host,
							},
							Weight: 100,
						},
					},
				},
			},
		},
	}

	if gateway != "" {
		vs.Spec.Gateways = []string{gateway}
	}

	_, err := c.sharedclientset.NetworkingV1alpha3().VirtualServices(vs.Namespace).Create(vs)
	return err
}
