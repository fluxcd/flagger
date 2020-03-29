package router

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// KubernetesNoopRouter manages nothing. This is useful when one uses Flagger for progressive delivery of
// services that are not load-balanced by a Kubernetes service
type KubernetesNoopRouter struct {
}

func (c *KubernetesNoopRouter) Initialize(_ *flaggerv1.Canary) error {
	return nil
}

func (c *KubernetesNoopRouter) Reconcile(_ *flaggerv1.Canary) error {
	return nil
}

func (c *KubernetesNoopRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
