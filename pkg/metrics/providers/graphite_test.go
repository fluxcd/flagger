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

package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewGraphiteProvider(t *testing.T) {
	addr := "http://graphite:8080"
	graph, err := NewGraphiteProvider(flaggerv1.MetricTemplateProvider{
		Address: addr,
	})

	require.NoError(t, err)
	assert.Equal(t, addr, graph.url.String())
}

func TestNewGraphiteProvider_InvalidURL(t *testing.T) {
	addr := ":::"
	_, err := NewGraphiteProvider(flaggerv1.MetricTemplateProvider{
		Address: addr,
		Type:    "graphite",
	})

	require.Error(t, err)
	assert.Equal(t, err.Error(), fmt.Sprintf("graphite address %s is not a valid URL", addr))
}
