package router

import (
	"strings"
)

const toolkitMarker = "toolkit.fluxcd.io"

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
	res := make(map[string]string)
	for k, v := range meta {
		if strings.Contains(k, toolkitMarker) {
			continue
		}
		res[k] = v
	}
	return res
}
