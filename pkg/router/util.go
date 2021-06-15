package router

import (
	"strings"
)

func includeLabelsByPrefix(labels map[string]string, includeLabelPrefixes []string) map[string]string {
	filteredLabels := make(map[string]string)
	for key, value := range labels {
		for _, includeLabelPrefix := range includeLabelPrefixes {
			if includeLabelPrefix == "*" || strings.HasPrefix(key, includeLabelPrefix) {
				filteredLabels[key] = value
				break
			}
		}
	}

	return filteredLabels
}

func makeAnnotations(in map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range in {
		// skip Flux GC markers
		if strings.Contains(key, "/checksum") {
			continue
		}
		out[key] = value
	}
	return out
}
