package canary

import (
	"crypto/rand"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
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
		if k != idKey {
			res[k] = v
		}
	}
	res[idKey] = id

	return res, nil
}

func makePrimaryLabels(labels map[string]string, primaryName string, label string) map[string]string {
	res := make(map[string]string)
	for k, v := range labels {
		if k != label {
			res[k] = v
		}
	}
	res[label] = primaryName

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
