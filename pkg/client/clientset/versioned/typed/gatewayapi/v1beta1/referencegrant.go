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

// Code generated by client-gen. DO NOT EDIT.

package v1beta1

import (
	"context"

	v1beta1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1beta1"
	scheme "github.com/fluxcd/flagger/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// ReferenceGrantsGetter has a method to return a ReferenceGrantInterface.
// A group's client should implement this interface.
type ReferenceGrantsGetter interface {
	ReferenceGrants(namespace string) ReferenceGrantInterface
}

// ReferenceGrantInterface has methods to work with ReferenceGrant resources.
type ReferenceGrantInterface interface {
	Create(ctx context.Context, referenceGrant *v1beta1.ReferenceGrant, opts v1.CreateOptions) (*v1beta1.ReferenceGrant, error)
	Update(ctx context.Context, referenceGrant *v1beta1.ReferenceGrant, opts v1.UpdateOptions) (*v1beta1.ReferenceGrant, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1beta1.ReferenceGrant, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1beta1.ReferenceGrantList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta1.ReferenceGrant, err error)
	ReferenceGrantExpansion
}

// referenceGrants implements ReferenceGrantInterface
type referenceGrants struct {
	*gentype.ClientWithList[*v1beta1.ReferenceGrant, *v1beta1.ReferenceGrantList]
}

// newReferenceGrants returns a ReferenceGrants
func newReferenceGrants(c *GatewayapiV1beta1Client, namespace string) *referenceGrants {
	return &referenceGrants{
		gentype.NewClientWithList[*v1beta1.ReferenceGrant, *v1beta1.ReferenceGrantList](
			"referencegrants",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *v1beta1.ReferenceGrant { return &v1beta1.ReferenceGrant{} },
			func() *v1beta1.ReferenceGrantList { return &v1beta1.ReferenceGrantList{} }),
	}
}