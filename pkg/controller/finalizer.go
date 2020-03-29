package controller

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

const finalizer = "finalizer.flagger.app"

func (c *Controller) finalize(old interface{}) error {

	r, ok := old.(*flaggerv1.Canary)
	if !ok {
		return fmt.Errorf("received unexpected object: %v", old)
	}

	_, err := c.flaggerClient.FlaggerV1beta1().Canaries(r.Namespace).Get(r.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get query error: %w", err)
	}

	// Retrieve a controller
	canaryController := c.canaryFactory.Controller(r.Spec.TargetRef.Kind)

	// Set the status to terminating if not already in that state
	if r.Status.Phase != flaggerv1.CanaryPhaseTerminating {
		if err := canaryController.SetStatusPhase(r, flaggerv1.CanaryPhaseTerminating); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

		// record event
		c.recordEventInfof(r, "Terminating canary %s.%s", r.Name, r.Namespace)
	}

	err = canaryController.Finalize(r)
	if err != nil {
		return fmt.Errorf("failed to revert target: %w", err)
	}
	c.logger.Infof("%s.%s kind %s reverted", r.Name, r.Namespace, r.Spec.TargetRef.Kind)

	// Ensure that targetRef has met a ready state
	c.logger.Infof("Checking is canary is ready %s.%s", r.Name, r.Namespace)
	_, err = canaryController.IsCanaryReady(r)
	if err != nil {
		return fmt.Errorf("canary not ready during finalizing: %w", err)
	}

	c.logger.Infof("%s.%s moving forward with router finalizing", r.Name, r.Namespace)
	labelSelector, ports, err := canaryController.GetMetadata(r)
	if err != nil {
		return fmt.Errorf("failed to get metadata for router finalizing: %w", err)
	}

	// Revert the router
	router := c.routerFactory.KubernetesRouter(r.Spec.TargetRef.Kind, labelSelector, map[string]string{}, ports)
	if err := router.Finalize(r); err != nil {
		return fmt.Errorf("failed revert router: %w", err)
	}
	c.logger.Infof("%s.%s router reverted", r.Name, r.Namespace)

	// TODO if I can't revert the mesh continue on?
	// Revert the Mesh
	if err := c.revertMesh(r); err != nil {
		return fmt.Errorf("failed to revert mesh: %w", err)
	}

	c.logger.Infof("Finalization complete for %s.%s", r.Name, r.Namespace)
	return nil
}

// revertMesh reverts defined mesh provider based upon the implementation's respective Finalize method.
// If the Finalize method encounters and error that is returned, else revert is considered successful.
func (c *Controller) revertMesh(r *flaggerv1.Canary) error {
	provider := c.meshProvider
	if r.Spec.Provider != "" {
		provider = r.Spec.Provider
	}

	meshRouter := c.routerFactory.MeshRouter(provider)
	if err := meshRouter.Finalize(r); err != nil {
		return fmt.Errorf("meshRouter.Finlize failed: %w", err)
	}

	c.logger.Infof("%s.%s mesh provider %s reverted", r.Name, r.Namespace, provider)
	return nil
}

// hasFinalizer evaluates the finalizers of a given canary for for existence of a provide finalizer string.
// It returns a boolean, true if the finalizer is found false otherwise.
func hasFinalizer(canary *flaggerv1.Canary) bool {
	for _, f := range canary.ObjectMeta.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// addFinalizer adds a provided finalizer to the specified canary resource.
// If failures occur the error will be returned otherwise the action is deemed successful
// and error will be nil.
func (c *Controller) addFinalizer(canary *flaggerv1.Canary) error {
	firstTry := true
	name, ns := canary.GetName(), canary.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			canary, err = c.flaggerClient.FlaggerV1beta1().Canaries(ns).Get(name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}

		cCopy := canary.DeepCopy()
		cCopy.ObjectMeta.Finalizers = append(cCopy.ObjectMeta.Finalizers, finalizer)
		_, err = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Update(cCopy)
		firstTry = false
		return
	})

	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}

// removeFinalizer removes a provided finalizer to the specified canary resource.
// If failures occur the error will be returned otherwise the action is deemed successful
// and error will be nil.
func (c *Controller) removeFinalizer(canary *flaggerv1.Canary) error {
	firstTry := true
	name, ns := canary.GetName(), canary.GetNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			canary, err = c.flaggerClient.FlaggerV1beta1().Canaries(ns).Get(name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", name, ns, err)
			}
		}

		cCopy := canary.DeepCopy()

		nfs := make([]string, 0, len(cCopy.ObjectMeta.Finalizers))
		for _, item := range cCopy.ObjectMeta.Finalizers {
			if item != finalizer {
				nfs = append(nfs, item)
			}
		}

		if len(nfs) == 0 {
			nfs = nil
		}

		cCopy.ObjectMeta.Finalizers = nfs
		_, err = c.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Update(cCopy)
		firstTry = false
		return
	})

	if err != nil {
		return fmt.Errorf("failed after retries: %w", err)
	}
	return nil
}
