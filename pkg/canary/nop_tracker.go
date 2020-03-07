package canary

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// NopTracker no-operation tracker
type NopTracker struct{}

func (nt *NopTracker) GetTargetConfigs(*flaggerv1.Canary) (map[string]ConfigRef, error) {
	res := make(map[string]ConfigRef)
	return res, nil
}

func (nt *NopTracker) GetConfigRefs(*flaggerv1.Canary) (*map[string]string, error) {
	return nil, nil
}

func (nt *NopTracker) HasConfigChanged(*flaggerv1.Canary) (bool, error) {
	return false, nil
}

func (nt *NopTracker) CreatePrimaryConfigs(*flaggerv1.Canary, map[string]ConfigRef) error {
	return nil
}

func (nt *NopTracker) ApplyPrimaryConfigs(spec corev1.PodSpec, _ map[string]ConfigRef) corev1.PodSpec {
	return spec
}
