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

package controller

import "time"

// CanaryJob holds the reference to a canary deployment schedule
type CanaryJob struct {
	Name             string
	Namespace        string
	function         func(name string, namespace string)
	done             chan bool
	ticker           *time.Ticker
	analysisInterval time.Duration
}

// Start runs the canary analysis on a schedule
func (j CanaryJob) Start() {
	go func() {
		// run the infra bootstrap on job creation
		j.function(j.Name, j.Namespace)
		for {
			select {
			case <-j.ticker.C:
				j.function(j.Name, j.Namespace)
			case <-j.done:
				return
			}
		}
	}()
}

// Stop closes the job channel and stops the ticker
func (j CanaryJob) Stop() {
	close(j.done)
	j.ticker.Stop()
}

func (j CanaryJob) GetCanaryAnalysisInterval() time.Duration {
	return j.analysisInterval
}
