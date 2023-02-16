package router

import (
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"strings"
)

const (
	toolkitMarker         = "toolkit.fluxcd.io"
	toolkitReconcileKey   = "kustomize.toolkit.fluxcd.io/reconcile"
	toolkitReconcileValue = "disabled"
)

func includeLabelsByPrefix(labels map[string]string, includeLabelPrefixes []string) map[string]string {
	filteredLabels := make(map[string]string)
	for key, value := range labels {
		if strings.Contains(key, toolkitMarker) {
			continue
		}
		for _, includeLabelPrefix := range includeLabelPrefixes {
			if includeLabelPrefix == "*" || strings.HasPrefix(key, includeLabelPrefix) {
				filteredLabels[key] = value
				break
			}
		}
	}

	return filteredLabels
}

func filterMetadata(meta map[string]string) map[string]string {
	// prevent Flux from overriding Flagger managed objects
	meta[toolkitReconcileKey] = toolkitReconcileValue
	return meta
}

// initializationWeights returns the initial weights that should be used to initialize
// router resources depending on whether progressive initialization is enabled
func initializationWeights(canary *flaggerv1.Canary) (
	initialPrimaryWeight int,
	initialCanaryWeight int,
) {
	if canary.ProgressiveInitialization() {
		return 0, 100
	} else {
		return 100, 0
	}
}
