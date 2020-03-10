package controller

import (
	"fmt"

	ex "github.com/pkg/errors"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const finalizer = "finalizer.flagger.app"

func (c *Controller) finalize(old interface{}) error {
	var r *flaggerv1.Canary
	var ok bool

	//Ensure interface is a canary
	if r, ok = old.(*flaggerv1.Canary); !ok {
		c.logger.Warnf("Received unexpected object: %v", old)
		return nil
	}

	//Retrieve a controller
	canaryController := c.canaryFactory.Controller(r.Spec.TargetRef.Kind)

	//Set the status to terminating if not already in that state
	if r.Status.Phase != flaggerv1.CanaryPhaseTerminating {
		if err := canaryController.SetStatusPhase(r, flaggerv1.CanaryPhaseTerminating); err != nil {
			c.logger.Infof("Failed to update status to finalizing %s", err)
			return err
		}
		//record event
		c.recordEventInfof(r, "Terminating canary %s.%s", r.Name, r.Namespace)
	}

	err := c.revertTargetRef(canaryController, r)
	if err != nil {
		if errors.IsNotFound(err) {
			//No reason to wait not found
			c.logger.Warnf("%s.%s failed due to %s not found", r.Name, r.Namespace, r.Spec.TargetRef.Kind)
			return nil
		}
		c.logger.Errorf("%s.%s failed due to %s", r.Name, r.Namespace, err)
		return err
	} else {
		//Ensure that targetRef has met a ready state
		c.logger.Infof("Checking is canary is ready %s.%s", r.Name, r.Namespace)
		ready, err := canaryController.IsCanaryReady(r)
		if err != nil && ready {
			return fmt.Errorf("%s.%s has not reached ready state during finalizing", r.Name, r.Namespace)
		}

	}

	c.logger.Infof("%s.%s moving forward with router finalizing", r.Name, r.Namespace)
	labelSelector, ports, err := canaryController.GetMetadata(r)
	if err != nil {
		c.logger.Errorf("%s.%s failed to get metadata for router finalizing", r.Name, r.Namespace)
		return err
	}
	//Revert the router
	if err := c.revertRouter(r, labelSelector, ports); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	c.logger.Infof("%s.%s moving forward with mesh finalizing", r.Name, r.Namespace)
	//TODO if I can't revert the mesh continue on?
	//Revert the Mesh
	if err := c.revertMesh(r); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	c.logger.Infof("Finalization complete for %s.%s", r.Name, r.Namespace)

	return nil
}

func (c *Controller) revertTargetRef(ctrl canary.Controller, r *flaggerv1.Canary) error {
	if err := ctrl.Finalize(r); err != nil {
		return err
	}
	c.logger.Infof("%s.%s kind %s reverted", r.Name, r.Namespace, r.Spec.TargetRef.Kind)
	return nil
}

//revertRouter
func (c *Controller) revertRouter(r *flaggerv1.Canary, labelSelector string, ports map[string]int32) error {
	router := c.routerFactory.KubernetesRouter(r.Spec.TargetRef.Kind, labelSelector, map[string]string{}, ports)
	if err := router.Finalize(r); err != nil {
		c.logger.Errorf("%s.%s router failed with error %s", r.Name, r.Namespace, err)
		return err
	}
	c.logger.Infof("Service %s.%s reverted", r.Name, r.Namespace)
	return nil
}

//revertMesh reverts defined mesh provider based upon the implementation's respective Finalize method.
//If the Finalize method encounters and error that is returned, else revert is considered successful.
func (c *Controller) revertMesh(r *flaggerv1.Canary) error {
	provider := c.meshProvider
	if r.Spec.Provider != "" {
		provider = r.Spec.Provider
	}

	//Establish provider
	meshRouter := c.routerFactory.MeshRouter(provider)

	//Finalize mesh
	err := meshRouter.Finalize(r)
	if err != nil {
		c.logger.Errorf("%s.%s mesh failed with error %s", r.Name, r.Namespace, err)
		return err
	}

	c.logger.Infof("%s.%s mesh provider %s reverted", r.Name, r.Namespace, provider)
	return nil
}

//hasFinalizer evaluates the finalizers of a given canary for for existence of a provide finalizer string.
//It returns a boolean, true if the finalizer is found false otherwise.
func hasFinalizer(canary *flaggerv1.Canary, finalizerString string) bool {
	currentFinalizers := canary.ObjectMeta.Finalizers

	for _, f := range currentFinalizers {
		if f == finalizerString {
			return true
		}
	}
	return false
}

//addFinalizer adds a provided finalizer to the specified canary resource.
//If failures occur the error will be returned otherwise the action is deemed successful
//and error will be nil.
func (c *Controller) addFinalizer(canary *flaggerv1.Canary, finalizerString string) error {
	firstTry := true
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {

		var selErr error
		if !firstTry {
			canary, selErr = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(canary.GetName(), metav1.GetOptions{})
			if selErr != nil {
				return selErr
			}
		}

		copy := canary.DeepCopy()
		copy.ObjectMeta.Finalizers = append(copy.ObjectMeta.Finalizers, finalizerString)

		_, err = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Update(copy)

		firstTry = false
		return
	})

	if err != nil {
		return ex.Wrap(err, "Remove finalizer failed")
	}
	return nil
}

//removeFinalizer removes a provided finalizer to the specified canary resource.
//If failures occur the error will be returned otherwise the action is deemed successful
//and error will be nil.
func (c *Controller) removeFinalizer(canary *flaggerv1.Canary, finalizerString string) error {
	firstTry := true
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {

		var selErr error
		if !firstTry {
			canary, selErr = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(canary.GetName(), metav1.GetOptions{})
			if selErr != nil {
				return selErr
			}
		}
		copy := canary.DeepCopy()

		newSlice := make([]string, 0)
		for _, item := range copy.ObjectMeta.Finalizers {
			if item == finalizerString {
				continue
			}
			newSlice = append(newSlice, item)
		}
		if len(newSlice) == 0 {
			newSlice = nil
		}
		copy.ObjectMeta.Finalizers = newSlice

		_, err = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Update(copy)

		firstTry = false
		return
	})

	if err != nil {
		return ex.Wrap(err, "Remove finalizer failed")
	}
	return nil
}
