package router

import flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"

type Interface interface {
	Reconcile(canary *flaggerv1.Canary) error
	SetRoutes(canary *flaggerv1.Canary, primaryWeight int, canaryWeight int) error
	GetRoutes(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, err error)
}
