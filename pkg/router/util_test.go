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

package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		"foo":                                  "foo-value",
		"bar":                                  "bar-value",
		"lorem":                                "ipsum",
		"kustomize.toolkit.fluxcd.io/checksum": "some",
	}
	includeLabelPrefix := []string{"*"}

	filteredLabels := includeLabelsByPrefix(labels, includeLabelPrefix)

	assert.Equal(t, filteredLabels, map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	})
}
