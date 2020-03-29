package router

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// NopRouter no-operation router
type NopRouter struct {
}

func (*NopRouter) Reconcile(_ *flaggerv1.Canary) error {
	return nil
}

func (*NopRouter) SetRoutes(_ *flaggerv1.Canary, _ int, _ int, _ bool) error {
	return nil
}

func (*NopRouter) GetRoutes(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, mirror bool, err error) {
	if canary.Status.Iterations > 0 {
		return 0, 100, false, nil
	}
	return 100, 0, false, nil
}

func (c *NopRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
