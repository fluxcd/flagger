package router

import flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"

const configAnnotation = "flagger.kubernetes.io/original-configuration"
const kubectlAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

type Interface interface {
	Reconcile(canary *flaggerv1.Canary) error
	SetRoutes(canary *flaggerv1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error
	GetRoutes(canary *flaggerv1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, err error)
	Finalize(canary *flaggerv1.Canary) error
}
