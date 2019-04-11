package controller

import (
	"fmt"
	"time"

	"github.com/solo-io/solo-kit/pkg/utils/log"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// This custom Kubernetes controller is used to provide a shared caching mechanism for the solo-kit resource clients.
type Controller struct {
	name string

	informers []cache.SharedIndexInformer

	// WorkQueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workQueue workqueue.RateLimitingInterface

	// handler to call
	handler cache.ResourceEventHandler
}

// Returns a new kubernetes controller without starting it.
func NewController(
	controllerName string,
	handler cache.ResourceEventHandler,
	informers ...cache.SharedIndexInformer) *Controller {

	return &Controller{
		name:      controllerName,
		informers: informers,
		workQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerName),
		handler:   handler,
	}
}

// Starts the controller by:
//
// 1. Registering the event handler with each if the informers
// 2. Starting each informer
// 3. Wait for the informer caches to sync
// 4. Starting a number of parallel workers equal to the "parallelism" parameter
//
// When stopCh is closed, the controller will stop the informers, shutdown the work queue and
// wait for workers to finish processing their current work items.
func (c *Controller) Run(parallelism int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()

	log.Debugf("Starting %v controller", c.name)

	// For each informer
	var syncFunctions []cache.InformerSynced
	for _, informer := range c.informers {

		// 1. Get the function to tell if it has synced
		syncFunctions = append(syncFunctions, informer.HasSynced)

		// 2. Register the event handler with the informer
		informer.AddEventHandler(c.eventHandlerFunctions())

		// 3. Run the informer
		go informer.Run(stopCh)
	}

	// Wait for all the informer caches to be synced before starting workers
	log.Debugf("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, []cache.InformerSynced(syncFunctions)...); !ok {
		return fmt.Errorf("error while waiting for caches to sync")
	}

	// Start workers in goroutine so we can defer the queue shutdown
	go func() {
		defer c.workQueue.ShutDown()
		log.Debugf("Starting workers")

		// Launch parallel workers to process resources
		for i := 0; i < parallelism; i++ {

			// WaitUntil internally defers a HandleCrash() before invoking runWorker()
			go wait.Until(c.runWorker, time.Second, stopCh)
		}
		log.Debugf("Started workers")

		<-stopCh
		log.Debugf("Stopping workers")
	}()

	return nil
}

// runWorker is a long-running function that will continually call the processNextWorkItem function
// in order to read and process a message on the work queue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the work queue and attempt to process it
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workQueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workQueue.Done(obj)
		var w *event
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if w, ok = obj.(*event); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workQueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected event type in workqueue but got %#v", obj))
			return nil
		}
		switch w.eventType {
		case added:
			c.handler.OnAdd(w.new)
		case updated:
			c.handler.OnUpdate(w.old, w.new)
		case deleted:
			c.handler.OnDelete(w.new)
		}

		c.workQueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
	}

	return true
}

// The resource event handler that will be registered with each of the controller's informers.
// When a resource changes, the informer will invoke the appropriate function to add an item to the work queue.
func (c *Controller) eventHandlerFunctions() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.enqueueSync(added, nil, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			c.enqueueSync(updated, old, new)
		},
		DeleteFunc: func(obj interface{}) {
			c.enqueueSync(deleted, nil, obj)
		},
	}
}

// Adds events to the work queue
func (c *Controller) enqueueSync(t eventType, old, new interface{}) {
	e := &event{
		eventType: t,
		old:       old,
		new:       new,
	}
	// log the meta key for the obj
	// currently unused otherwise
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(e.new); err != nil {
		runtime.HandleError(err)
		return
	}
	// TODO: create multiple verbosity levels
	if false {
		log.Debugf("[%s] EVENT: %s: %s", c.name, e.eventType, key)
	}
	c.workQueue.AddRateLimited(e)
}
