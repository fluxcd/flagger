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

type Interface interface {
	// RunQuery executes the query and converts the first result to float64
	RunQuery(query string) (float64, error)

	// IsOnline calls the provider endpoint and returns an error if the API is unreachable
	IsOnline() (bool, error)
}
