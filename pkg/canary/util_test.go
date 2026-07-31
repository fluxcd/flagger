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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestIncludeLabelsByPrefix(t *testing.T) {
	labels := map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	}
	includeLabelPrefix := []string{"foo", "lor"}

	filteredLabels := includeLabelsByPrefix(labels, includeLabelPrefix)

	assert.Equal(t, filteredLabels, map[string]string{
		"foo":   "foo-value",
		"lorem": "ipsum",
		// bar excluded
	})
}

func TestIncludeLabelsByPrefixWithWildcard(t *testing.T) {
	labels := map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	}
	includeLabelPrefix := []string{"*"}

	filteredLabels := includeLabelsByPrefix(labels, includeLabelPrefix)

	assert.Equal(t, filteredLabels, map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	})
}

func TestIncludeLabelsNoIncludes(t *testing.T) {
	labels := map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	}
	includeLabelPrefix := []string{""}

	filteredLabels := includeLabelsByPrefix(labels, includeLabelPrefix)

	assert.Equal(t, map[string]string{}, filteredLabels)
}

func TestGetPortsExcludesSidecars(t *testing.T) {
	cd := &flaggerv1.Canary{
		Spec: flaggerv1.CanarySpec{
			Service: flaggerv1.CanaryService{
				Port: 8080,
			},
		},
	}

	containers := []corev1.Container{
		{
			Name: "app",
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8080},
				{Name: "metrics", ContainerPort: 9090},
			},
		},
		{
			Name: "linkerd-proxy",
			Ports: []corev1.ContainerPort{
				{Name: "linkerd-proxy", ContainerPort: 4143},
				{Name: "linkerd-admin", ContainerPort: 4191},
			},
		},
		{
			Name: "istio-proxy",
			Ports: []corev1.ContainerPort{
				{Name: "istio-proxy", ContainerPort: 15090},
			},
		},
	}

	ports := getPorts(cd, containers)

	assert.Equal(t, map[string]int32{"metrics": 9090}, ports)
}

func TestMakePrimaryLabels(t *testing.T) {
	labels := map[string]string{
		"lorem": "ipsum",
		"foo":   "old-bar",
	}

	primaryLabels := makePrimaryLabels(labels, "new-bar", "foo")

	assert.Equal(t, primaryLabels, map[string]string{
		"lorem": "ipsum",   // values from old map
		"foo":   "new-bar", // overriden value for a specific label
	})
}
