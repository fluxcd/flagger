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
	"net/url"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// GraphiteProvider executes Graphite queries.
type GraphiteProvider struct {
	url url.URL
}

// NewGraphiteProvider takes a provider spec and returns a Graphite
// client ready to execute queries against the API.
func NewGraphiteProvider(provider flaggerv1.MetricTemplateProvider) (*GraphiteProvider, error) {
	graphiteURL, err := url.Parse(provider.Address)
	if provider.Address == "" || err != nil {
		return nil, fmt.Errorf("%s address %s is not a valid URL", provider.Type, provider.Address)
	}

	graph := GraphiteProvider{
		url: *graphiteURL,
	}

	return &graph, nil
}
