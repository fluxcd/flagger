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

package observers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func Test_RenderQuery(t *testing.T) {
	t.Run("ok_without_variables", func(t *testing.T) {
		expected := `sum(envoy_cluster_upstream_rq{envoy_cluster_name=~"default_myapp"})`
		templateQuery := `sum(envoy_cluster_upstream_rq{envoy_cluster_name=~"{{ namespace }}_{{ target }}"})`

		model := &flaggerv1.MetricTemplateModel{
			Name:      "standard",
			Namespace: "default",
			Target:    "myapp",
			Interval:  "1m",
		}

		actual, err := RenderQuery(templateQuery, *model)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	})

	t.Run("ok_with_variables", func(t *testing.T) {
		expected := `delta(max by (consumer_group) (kafka_consumer_current_offset{cluster="dev", consumer_group="my_consumer"}[1m]))`
		templateQuery := `delta(max by (consumer_group) (kafka_consumer_current_offset{cluster="{{ variables.cluster }}", consumer_group="{{ variables.consumer_group }}"}[{{ interval }}]))`

		model := &flaggerv1.MetricTemplateModel{
			Name:      "kafka_consumer_offset",
			Namespace: "default",
			Interval:  "1m",
			Variables: map[string]string{
				"cluster":        "dev",
				"consumer_group": "my_consumer",
			},
		}

		actual, err := RenderQuery(templateQuery, *model)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	})

	t.Run("missing_variable_key", func(t *testing.T) {
		templateQuery := `delta(max by (consumer_group) (kafka_consumer_current_offset{cluster="{{ variables.cluster }}", consumer_group="{{ variables.consumer_group }}"}[{{ interval }}]))`

		model := &flaggerv1.MetricTemplateModel{
			Name:      "kafka_consumer_offset",
			Namespace: "default",
			Interval:  "1m",
			Variables: map[string]string{
				"invalid":        "dev",
				"consumer_group": "my_consumer",
			},
		}

		_, err := RenderQuery(templateQuery, *model)
		require.Error(t, err)
	})
}
