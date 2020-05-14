/*
Copyright The Flagger Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package v1beta2

import (
	v1beta2 "github.com/weaveworks/flagger/pkg/apis/appmesh/v1beta2"
	"github.com/weaveworks/flagger/pkg/client/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

type AppmeshV1beta2Interface interface {
	RESTClient() rest.Interface
	VirtualNodesGetter
	VirtualRoutersGetter
	VirtualServicesGetter
}

// AppmeshV1beta2Client is used to interact with features provided by the appmesh.k8s.aws group.
type AppmeshV1beta2Client struct {
	restClient rest.Interface
}

func (c *AppmeshV1beta2Client) VirtualNodes(namespace string) VirtualNodeInterface {
	return newVirtualNodes(c, namespace)
}

func (c *AppmeshV1beta2Client) VirtualRouters(namespace string) VirtualRouterInterface {
	return newVirtualRouters(c, namespace)
}

func (c *AppmeshV1beta2Client) VirtualServices(namespace string) VirtualServiceInterface {
	return newVirtualServices(c, namespace)
}

// NewForConfig creates a new AppmeshV1beta2Client for the given config.
func NewForConfig(c *rest.Config) (*AppmeshV1beta2Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &AppmeshV1beta2Client{client}, nil
}

// NewForConfigOrDie creates a new AppmeshV1beta2Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *AppmeshV1beta2Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new AppmeshV1beta2Client for the given RESTClient.
func New(c rest.Interface) *AppmeshV1beta2Client {
	return &AppmeshV1beta2Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1beta2.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *AppmeshV1beta2Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}