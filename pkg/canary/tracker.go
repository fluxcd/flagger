package canary

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

type Tracker interface {
	GetTargetConfigs(cd *flaggerv1.Canary) (map[string]ConfigRef, error)
	GetConfigRefs(cd *flaggerv1.Canary) (*map[string]string, error)
	HasConfigChanged(cd *flaggerv1.Canary) (bool, error)
	CreatePrimaryConfigs(cd *flaggerv1.Canary, refs map[string]ConfigRef) error
	ApplyPrimaryConfigs(spec corev1.PodSpec, refs map[string]ConfigRef) corev1.PodSpec
}
