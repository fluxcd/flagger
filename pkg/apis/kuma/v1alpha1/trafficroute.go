/*
Copyright 2019 Kuma authors.
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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced

// TrafficRoute is the Schema for the Traffic Routes API.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TrafficRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Mesh              string           `json:"mesh,omitempty"`
	Spec              TrafficRouteSpec `json:"spec,omitempty"`
}

// TrafficRouteList defines a list of TrafficRoute objects.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TrafficRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficRoute `json:"items"`
}

// TrafficRouteSpec defines the spec for a TrafficRoute.
type TrafficRouteSpec struct {
	// List of selectors to match data plane proxies that are sources of traffic.
	Sources []*Selector `json:"sources,omitempty"`
	// List of selectors to match services that are destinations of traffic.
	//
	// Notice the difference between sources and destinations.
	// While the source of traffic is always a data plane proxy within a mesh,
	// the destination is a service that could be either within or outside
	// of a mesh.
	Destinations []*Selector `json:"destinations,omitempty"`
	// Configuration for the route.
	Conf *TrafficRouteConf `json:"conf,omitempty"`
}

// Selector defines the configuration for which Kuma services should be targeted.
type Selector struct {
	// Tags to match, can be used for both source and destinations
	Match map[string]string `json:"match,omitempty"`
}

// TrafficRouteConf defines the destination configuration.
type TrafficRouteConf struct {
	// List of destinations with weights assigned to them.
	// When used, "destination" is not allowed.
	Split []*TrafficRouteSplit `json:"split,omitempty"`
}

// TrafficRouteSplit defines a destination with a weight assigned to it.
type TrafficRouteSplit struct {
	// Weight assigned to that destination.
	// Weights are not percentages. For example two destinations with
	// weights the same weight "1" will receive both same amount of the traffic.
	// 0 means that the destination will be ignored.
	Weight uint32 `json:"weight"`
	// Selector to match individual endpoints that comprise that destination.
	//
	// Notice that an endpoint can be either inside or outside the mesh.
	// In the former case an endpoint corresponds to a data plane proxy,
	// in the latter case an endpoint is an External Service.
	Destination map[string]string `json:"destination,omitempty"`
}
