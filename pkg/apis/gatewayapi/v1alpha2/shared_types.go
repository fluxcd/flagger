/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ParentReference identifies an API object (usually a Gateway) that can be considered
// a parent of this resource (usually a route). The only kind of parent resource
// with "Core" support is Gateway. This API may be extended in the future to
// support additional kinds of parent resources, such as HTTPRoute.
//
// The API object must be valid in the cluster; the Group and Kind must
// be registered in the cluster for this reference to be valid.
//
// References to objects with invalid Group and Kind are not valid, and must
// be rejected by the implementation, with appropriate Conditions set
// on the containing object.
type ParentReference struct {
	// Group is the group of the referent.
	//
	// Support: Core
	//
	// +kubebuilder:default=gateway.networking.k8s.io
	// +optional
	Group *Group `json:"group,omitempty"`

	// Kind is kind of the referent.
	//
	// Support: Core (Gateway)
	// Support: Custom (Other Resources)
	//
	// +kubebuilder:default=Gateway
	// +optional
	Kind *Kind `json:"kind,omitempty"`

	// Namespace is the namespace of the referent. When unspecified (or empty
	// string), this refers to the local namespace of the Route.
	//
	// Support: Core
	//
	// +optional
	Namespace *Namespace `json:"namespace,omitempty"`

	// Name is the name of the referent.
	//
	// Support: Core
	Name ObjectName `json:"name"`

	// SectionName is the name of a section within the target resource. In the
	// following resources, SectionName is interpreted as the following:
	//
	// * Gateway: Listener Name
	//
	// Implementations MAY choose to support attaching Routes to other resources.
	// If that is the case, they MUST clearly document how SectionName is
	// interpreted.
	//
	// When unspecified (empty string), this will reference the entire resource.
	// For the purpose of status, an attachment is considered successful if at
	// least one section in the parent resource accepts it. For example, Gateway
	// listeners can restrict which Routes can attach to them by Route kind,
	// namespace, or hostname. If 1 of 2 Gateway listeners accept attachment from
	// the referencing Route, the Route MUST be considered successfully
	// attached. If no Gateway listeners accept attachment from this Route, the
	// Route MUST be considered detached from the Gateway.
	//
	// Support: Core
	//
	// +optional
	SectionName *SectionName `json:"sectionName,omitempty"`
}

// CommonRouteSpec defines the common attributes that all Routes MUST include
// within their spec.
type CommonRouteSpec struct {
	// ParentRefs references the resources (usually Gateways) that a Route wants
	// to be attached to. Note that the referenced parent resource needs to
	// allow this for the attachment to be complete. For Gateways, that means
	// the Gateway needs to allow attachment from Routes of this kind and
	// namespace.
	//
	// The only kind of parent resource with "Core" support is Gateway. This API
	// may be extended in the future to support additional kinds of parent
	// resources such as one of the route kinds.
	//
	// It is invalid to reference an identical parent more than once. It is
	// valid to reference multiple distinct sections within the same parent
	// resource, such as 2 Listeners within a Gateway.
	//
	// It is possible to separately reference multiple distinct objects that may
	// be collapsed by an implementation. For example, some implementations may
	// choose to merge compatible Gateway Listeners together. If that is the
	// case, the list of routes attached to those resources should also be
	// merged.
	//
	// +optional
	// +kubebuilder:validation:MaxItems=32
	ParentRefs []ParentReference `json:"parentRefs,omitempty"`
}

// BackendRef defines how a Route should forward a request to a Kubernetes
// resource.
//
// Note that when a namespace is specified, a ReferencePolicy object
// is required in the referent namespace to allow that namespace's
// owner to accept the reference. See the ReferencePolicy documentation
// for details.
type BackendRef struct {
	// BackendObjectReference references a Kubernetes object.
	BackendObjectReference `json:",inline"`

	// Weight specifies the proportion of requests forwarded to the referenced
	// backend. This is computed as weight/(sum of all weights in this
	// BackendRefs list). For non-zero values, there may be some epsilon from
	// the exact proportion defined here depending on the precision an
	// implementation supports. Weight is not a percentage and the sum of
	// weights does not need to equal 100.
	//
	// If only one backend is specified and it has a weight greater than 0, 100%
	// of the traffic is forwarded to that backend. If weight is set to 0, no
	// traffic should be forwarded for this entry. If unspecified, weight
	// defaults to 1.
	//
	// Support for this field varies based on the context where used.
	//
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000000
	Weight *int32 `json:"weight,omitempty"`
}

// RouteConditionType is a type of condition for a route.
type RouteConditionType string

const (
	// This condition indicates whether the route has been accepted or rejected
	// by a Gateway, and why.
	ConditionRouteAccepted RouteConditionType = "Accepted"

	// This condition indicates whether the controller was able to resolve all
	// the object references for the Route.
	ConditionRouteResolvedRefs RouteConditionType = "ResolvedRefs"
)

// RouteParentStatus describes the status of a route with respect to an
// associated Parent.
type RouteParentStatus struct {
	// ParentRef corresponds with a ParentRef in the spec that this
	// RouteParentStatus struct describes the status of.
	ParentRef ParentReference `json:"parentRef"`

	// ControllerName is a domain/path string that indicates the name of the
	// controller that wrote this status. This corresponds with the
	// controllerName field on GatewayClass.
	//
	// Example: "example.net/gateway-controller".
	//
	// The format of this field is DOMAIN "/" PATH, where DOMAIN and PATH are
	// valid Kubernetes names
	// (https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names).
	ControllerName GatewayController `json:"controllerName"`

	// Conditions describes the status of the route with respect to the Gateway.
	// Note that the route's availability is also subject to the Gateway's own
	// status conditions and listener status.
	//
	// If the Route's ParentRef specifies an existing Gateway that supports
	// Routes of this kind AND that Gateway's controller has sufficient access,
	// then that Gateway's controller MUST set the "Accepted" condition on the
	// Route, to indicate whether the route has been accepted or rejected by the
	// Gateway, and why.
	//
	// A Route MUST be considered "Accepted" if at least one of the Route's
	// rules is implemented by the Gateway.
	//
	// There are a number of cases where the "Accepted" condition may not be set
	// due to lack of controller visibility, that includes when:
	//
	// * The Route refers to a non-existent parent.
	// * The Route is of a type that the controller does not support.
	// * The Route is in a namespace the the controller does not have access to.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RouteStatus defines the common attributes that all Routes MUST include within
// their status.
type RouteStatus struct {
	// Parents is a list of parent resources (usually Gateways) that are
	// associated with the route, and the status of the route with respect to
	// each parent. When this route attaches to a parent, the controller that
	// manages the parent must add an entry to this list when the controller
	// first sees the route and should update the entry as appropriate when the
	// route or gateway is modified.
	//
	// Note that parent references that cannot be resolved by an implementation
	// of this API will not be added to this list. Implementations of this API
	// can only populate Route status for the Gateways/parent resources they are
	// responsible for.
	//
	// A maximum of 32 Gateways will be represented in this list. An empty list
	// means the route has not been attached to any Gateway.
	//
	// +kubebuilder:validation:MaxItems=32
	Parents []RouteParentStatus `json:"parents"`
}

// PreciseHostname is the fully qualified domain name of a network host. This matches
// the RFC 1123 definition of a hostname with 1 notable exception that
// numeric IP addresses are not allowed.
//
// PreciseHostname can be "precise" which is a domain name without the terminating
// dot of a network host (e.g. "foo.example.com").
//
// Note that as per RFC1035 and RFC1123, a *label* must consist of lower case
// alphanumeric characters or '-', and must start and end with an alphanumeric
// character. No other punctuation is allowed.
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
type PreciseHostname string

// Group refers to a Kubernetes Group. It must either be an empty string or a
// RFC 1123 subdomain.
//
// This validation is based off of the corresponding Kubernetes validation:
// https://github.com/kubernetes/apimachinery/blob/02cfb53916346d085a6c6c7c66f882e3c6b0eca6/pkg/util/validation/validation.go#L208
//
// Valid values include:
//
// * "" - empty string implies core Kubernetes API group
// * "networking.k8s.io"
// * "foo.example.com"
//
// Invalid values include:
//
// * "example.com/bar" - "/" is an invalid character
//
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
type Group string

// Kind refers to a Kubernetes Kind.
//
// Valid values include:
//
// * "Service"
// * "HTTPRoute"
//
// Invalid values include:
//
// * "invalid/kind" - "/" is an invalid character
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
type Kind string

// ObjectName refers to the name of a Kubernetes object.
// Object names can have a variety of forms, including RFC1123 subdomains,
// RFC 1123 labels, or RFC 1035 labels.
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
type ObjectName string

// Namespace refers to a Kubernetes namespace. It must be a RFC 1123 label.
//
// This validation is based off of the corresponding Kubernetes validation:
// https://github.com/kubernetes/apimachinery/blob/02cfb53916346d085a6c6c7c66f882e3c6b0eca6/pkg/util/validation/validation.go#L187
//
// This is used for Namespace name validation here:
// https://github.com/kubernetes/apimachinery/blob/02cfb53916346d085a6c6c7c66f882e3c6b0eca6/pkg/api/validation/generic.go#L63
//
// Valid values include:
//
// * "example"
//
// Invalid values include:
//
// * "example.com" - "." is an invalid character
//
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
type Namespace string

// SectionName is the name of a section in a Kubernetes resource.
//
// This validation is based off of the corresponding Kubernetes validation:
// https://github.com/kubernetes/apimachinery/blob/02cfb53916346d085a6c6c7c66f882e3c6b0eca6/pkg/util/validation/validation.go#L208
//
// Valid values include:
//
// * "example.com"
// * "foo.example.com"
//
// Invalid values include:
//
// * "example.com/bar" - "/" is an invalid character
//
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
type SectionName string

// GatewayController is the name of a Gateway API controller. It must be a
// domain prefixed path.
//
// Valid values include:
//
// * "example.com/bar"
//
// Invalid values include:
//
// * "example.com" - must include path
// * "foo.example.com" - must include path
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[A-Za-z0-9\/\-._~%!$&'()*+,;=:]+$`
type GatewayController string

// AddressRouteMatches defines AddressMatch rules for inbound traffic according to
// source and/or destination address of a packet or connection.
type AddressRouteMatches struct {
	// SourceAddresses indicates the originating (source) network
	// addresses which are valid for routing traffic.
	//
	// Support: Extended
	SourceAddresses []AddressMatch `json:"sourceAddresses"`

	// DestinationAddresses indicates the destination network addresses
	// which are valid for routing traffic.
	//
	// Support: Extended
	DestinationAddresses []AddressMatch `json:"destinationAddresses"`
}

// AddressMatch defines matching rules for network addresses by type.
type AddressMatch struct {
	// Type of the address, either IPAddress or NamedAddress.
	//
	// If NamedAddress is used this is a custom and specific value for each
	// implementation to handle (and add validation for) according to their
	// own needs.
	//
	// For IPAddress the implementor may expect either IPv4 or IPv6.
	//
	// Support: Core (IPAddress)
	// Support: Custom (NamedAddress)
	//
	// +optional
	// +kubebuilder:validation:Enum=IPAddress;NamedAddress
	// +kubebuilder:default=IPAddress
	Type *AddressType `json:"type,omitempty"`

	// Value of the address. The validity of the values will depend
	// on the type and support by the controller.
	//
	// If implementations support proxy-protocol (see:
	// https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt) they
	// must respect the connection metadata from proxy-protocol
	// in the match logic implemented for these address values.
	//
	// Examples: `1.2.3.4`, `128::1`, `my-named-address`.
	//
	// Support: Core
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Value string `json:"value"`
}

// AddressType defines how a network address is represented as a text string.
type AddressType string

const (
	// A textual representation of a numeric IP address. IPv4
	// addresses must be in dotted-decimal form. IPv6 addresses
	// must be in a standard IPv6 text representation
	// (see [RFC 5952](https://tools.ietf.org/html/rfc5952)).
	//
	// This type is intended for specific addresses. Address ranges are not
	// supported (e.g. you can not use a CIDR range like 127.0.0.0/24 as an
	// IPAddress).
	//
	// Support: Extended
	IPAddressType AddressType = "IPAddress"

	// A Hostname represents a DNS based ingress point. This is similar to the
	// corresponding hostname field in Kubernetes load balancer status. For
	// example, this concept may be used for cloud load balancers where a DNS
	// name is used to expose a load balancer.
	//
	// Support: Extended
	HostnameAddressType AddressType = "Hostname"

	// A NamedAddress provides a way to reference a specific IP address by name.
	// For example, this may be a name or other unique identifier that refers
	// to a resource on a cloud provider such as a static IP.
	//
	// Support: Implementation-Specific
	NamedAddressType AddressType = "NamedAddress"
)
