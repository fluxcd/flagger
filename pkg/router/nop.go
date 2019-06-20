package router

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
)

// NopRouter no-operation router
type NopRouter struct {
}

func (*NopRouter) Reconcile(canary *flaggerv1.Canary) error {
	return nil
}

func (*NopRouter) SetRoutes(canary *flaggerv1.Canary, primaryWeight int, canaryWeight int) error {
	return nil
}

func (*NopRouter) GetRoutes(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, err error) {
	if canary.Status.Iterations > 0 {
		return 0, 100, nil
	}
	return 100, 0, nil
}
