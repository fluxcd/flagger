package router

import (
	"strings"
)

const (
	toolkitMarker         = "toolkit.fluxcd.io"
	toolkitReconcileKey   = "kustomize.toolkit.fluxcd.io/reconcile"
	helmDriftDetectionKey = "helm.toolkit.fluxcd.io/driftDetection"
	toolkitReconcileValue = "disabled"
)

func includeLabelsByPrefix(labels map[string]string, includeLabelPrefixes []string) map[string]string {
	filteredLabels := make(map[string]string)
	for key, value := range labels {
		if strings.Contains(key, toolkitMarker) {
			continue
		}
		for _, includeLabelPrefix := range includeLabelPrefixes {
			if includeLabelPrefix == "*" || (includeLabelPrefix != "" && strings.HasPrefix(key, includeLabelPrefix)) {
				filteredLabels[key] = value
				break
			}
		}
	}

	return filteredLabels
}

func filterMetadata(meta map[string]string) map[string]string {
	if meta == nil {
		meta = map[string]string{}
	}
	// prevent Flux from overriding Flagger managed objects
	meta[toolkitReconcileKey] = toolkitReconcileValue
	meta[helmDriftDetectionKey] = toolkitReconcileValue
	return meta
}
