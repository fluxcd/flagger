/*
Copyright 2025 The Flux authors

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

package v1beta1

import (
	"testing"

	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1beta1 "github.com/fluxcd/flagger/pkg/apis/istio/v1beta1"
	"github.com/stretchr/testify/assert"
)

func TestCanary_GetDeploymentStrategy(t *testing.T) {
	tests := []struct {
		name     string
		analysis *CanaryAnalysis
		expected string
	}{
		{
			name: "canary strategy with maxWeight",
			analysis: &CanaryAnalysis{
				MaxWeight:  30,
				StepWeight: 10,
			},
			expected: DeploymentStrategyCanary,
		},
		{
			name: "canary strategy with stepWeights",
			analysis: &CanaryAnalysis{
				StepWeights: []int{10, 20, 30},
			},
			expected: DeploymentStrategyCanary,
		},
		{
			name: "blue-green strategy with iterations",
			analysis: &CanaryAnalysis{
				Iterations: 5,
			},
			expected: DeploymentStrategyBlueGreen,
		},
		{
			name: "ab-testing strategy with iterations and match",
			analysis: &CanaryAnalysis{
				Iterations: 10,
				Match: []istiov1beta1.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"x-canary": {
								Exact: "insider",
							},
						},
					},
				},
			},
			expected: DeploymentStrategyABTesting,
		},
		{
			name:     "default to canary when analysis is nil",
			analysis: nil,
			expected: DeploymentStrategyCanary,
		},
		{
			name:     "default to canary when analysis is empty",
			analysis: &CanaryAnalysis{},
			expected: DeploymentStrategyCanary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canary := &Canary{
				Spec: CanarySpec{
					Analysis: tt.analysis,
				},
			}
			result := canary.DeploymentStrategy()
			assert.Equal(t, tt.expected, result)
		})
	}
}
