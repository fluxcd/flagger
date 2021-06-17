/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package canary

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

var sidecars = map[string]bool{
	"istio-proxy": true,
	"envoy":       true,
}

func getPorts(cd *flaggerv1.Canary, cs []corev1.Container) map[string]int32 {
	ports := make(map[string]int32, len(cs))
	for _, container := range cs {
		// exclude service mesh proxies based on container name
		if _, ok := sidecars[container.Name]; ok {
			continue
		}
		for i, p := range container.Ports {
			// exclude canary.service.port or canary.service.targetPort
			if cd.Spec.Service.TargetPort.String() == "0" {
				if p.ContainerPort == cd.Spec.Service.Port {
					continue
				}
			} else {
				if cd.Spec.Service.TargetPort.Type == intstr.Int {
					if p.ContainerPort == cd.Spec.Service.TargetPort.IntVal {
						continue
					}
				}
				if cd.Spec.Service.TargetPort.Type == intstr.String {
					if p.Name == cd.Spec.Service.TargetPort.StrVal {
						continue
					}
				}
			}
			name := fmt.Sprintf("tcp-%s-%v", container.Name, i)
			if p.Name != "" {
				name = p.Name
			}

			ports[name] = p.ContainerPort
		}
	}
	return ports
}

const toolkitMarker = "toolkit.fluxcd.io"

// makeAnnotations appends an unique ID to annotations map
func makeAnnotations(annotations map[string]string) (map[string]string, error) {
	idKey := "flagger-id"
	res := make(map[string]string)
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return res, fmt.Errorf("%w", err)
	}
	uuid[8] = uuid[8]&^0xc0 | 0x80
	uuid[6] = uuid[6]&^0xf0 | 0x40
	id := fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])

	for k, v := range annotations {
		if strings.Contains(k, toolkitMarker) {
			continue
		}
		if k != idKey {
			res[k] = v
		}
	}
	res[idKey] = id

	return res, nil
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

func makePrimaryLabels(labels map[string]string, labelValue string, label string) map[string]string {
	res := make(map[string]string)
	for k, v := range labels {
		if strings.Contains(k, toolkitMarker) {
			continue
		}
		if k != label {
			res[k] = v
		}
	}
	res[label] = labelValue

	return res
}

func int32p(i int32) *int32 {
	return &i
}

func int32Default(i *int32) int32 {
	if i == nil {
		return 1
	}

	return *i
}
