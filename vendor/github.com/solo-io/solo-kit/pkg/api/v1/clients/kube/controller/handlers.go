package controller

import (
	"fmt"
	"log"
	"sync"

	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"

	"k8s.io/client-go/tools/cache"
)

// returns a handler that runs f() every time an update occurs,
// regardless of which type of update
func NewSyncHandler(f func()) cache.ResourceEventHandler {
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			f()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			f()
		},
		DeleteFunc: func(obj interface{}) {
			f()
		},
	}
}

// returns a handler that runs f() every time an update occurs,
// regardless of which type of update
// ensures only one f() can run at a time
func NewLockingSyncHandler(f func()) cache.ResourceEventHandler {
	var mu sync.Mutex
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			mu.Lock()
			f()
			mu.Unlock()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			mu.Lock()
			f()
			mu.Unlock()
		},
		DeleteFunc: func(obj interface{}) {
			mu.Lock()
			f()
			mu.Unlock()
		},
	}
}

// Returns a handler that runs the given function every time an update occurs, passing in the updated resource.
// Ensures that only one callback can run at a time.
func NewLockingCallbackHandler(callback func(v1.Resource)) cache.ResourceEventHandler {
	var mu sync.Mutex
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			mu.Lock()
			callback(toResource(obj))
			mu.Unlock()
		},
		// Disregard  the old version of the resource
		UpdateFunc: func(oldObj, newObj interface{}) {
			mu.Lock()
			callback(toResource(newObj))
			mu.Unlock()
		},
		DeleteFunc: func(obj interface{}) {
			mu.Lock()
			callback(toResource(obj))
			mu.Unlock()
		},
	}
}

func toResource(obj interface{}) v1.Resource {
	switch object := obj.(type) {
	case *v1.Resource:
		// Pointer should never be nil
		if object == nil {
			log.Panicf("unexpected nil resource received from kube controller: %v", object)
		}
		return *object
	default:
		// Should never happen, since our controller currently only handles informers for our resources
		panic(fmt.Sprintf("unsupported resource type received from kube controller: %v", obj))
	}
}
