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

package router

import (
	"context"
	"fmt"

	gloov1 "github.com/fluxcd/flagger/pkg/apis/gloo/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// GlooRouter is managing Istio virtual services
type GlooRouter struct {
	kubeClient          kubernetes.Interface
	glooClient          clientset.Interface
	flaggerClient       clientset.Interface
	logger              *zap.SugaredLogger
	upstreamDiscoveryNs string
}

// Reconcile creates or updates the Istio virtual service
func (gr *GlooRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, _, _ := canary.GetServiceNames()
	canaryName := fmt.Sprintf("%s-%s-canary-%v", canary.Namespace, apexName, canary.Spec.Service.Port)
	primaryName := fmt.Sprintf("%s-%s-primary-%v", canary.Namespace, apexName, canary.Spec.Service.Port)

	newSpec := gloov1.UpstreamGroupSpec{
		Destinations: []gloov1.WeightedDestination{
			{
				Destination: gloov1.Destination{
					Upstream: gloov1.ResourceRef{
						Name:      primaryName,
						Namespace: gr.upstreamDiscoveryNs,
					},
				},
				Weight: 100,
			},
			{
				Destination: gloov1.Destination{
					Upstream: gloov1.ResourceRef{
						Name:      canaryName,
						Namespace: gr.upstreamDiscoveryNs,
					},
				},
				Weight: 0,
			},
		},
	}

	upstreamGroup, err := gr.glooClient.GlooV1().UpstreamGroups(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		upstreamGroup = &gloov1.UpstreamGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      apexName,
				Namespace: canary.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: newSpec,
		}

		_, err = gr.glooClient.GlooV1().UpstreamGroups(canary.Namespace).Create(context.TODO(), upstreamGroup, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("UpstreamGroup %s.%s create error: %w", apexName, canary.Namespace, err)
		}
		gr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("UpstreamGroup %s.%s created", upstreamGroup.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("UpstreamGroup %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	// update upstreamGroup but keep the original destination weights
	if upstreamGroup != nil {
		if diff := cmp.Diff(
			newSpec,
			upstreamGroup.Spec,
			cmpopts.IgnoreFields(gloov1.WeightedDestination{}, "Weight"),
		); diff != "" {
			clone := upstreamGroup.DeepCopy()
			clone.Spec = newSpec

			_, err = gr.glooClient.GlooV1().UpstreamGroups(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("UpstreamGroup %s.%s update error: %w", apexName, canary.Namespace, err)
			}
			gr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("UpstreamGroup %s.%s updated", upstreamGroup.GetName(), canary.Namespace)
		}
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (gr *GlooRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName := canary.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-%s-primary-%v", canary.Namespace, canary.Spec.TargetRef.Name, canary.Spec.Service.Port)

	upstreamGroup, err := gr.glooClient.GlooV1().UpstreamGroups(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("UpstreamGroup %s.%s get query error: %w", apexName, canary.Namespace, err)
		return
	}

	if len(upstreamGroup.Spec.Destinations) < 2 {
		err = fmt.Errorf("UpstreamGroup %s.%s destinations not found", apexName, canary.Namespace)
		return
	}

	for _, dst := range upstreamGroup.Spec.Destinations {
		if dst.Destination.Upstream.Name == primaryName {
			primaryWeight = int(dst.Weight)
			canaryWeight = 100 - primaryWeight
			return
		}
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (gr *GlooRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, _, _ := canary.GetServiceNames()
	canaryName := fmt.Sprintf("%s-%s-canary-%v", canary.Namespace, apexName, canary.Spec.Service.Port)
	primaryName := fmt.Sprintf("%s-%s-primary-%v", canary.Namespace, apexName, canary.Spec.Service.Port)

	if primaryWeight == 0 && canaryWeight == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", apexName, canary.Namespace)
	}

	upstreamGroup, err := gr.glooClient.GlooV1().UpstreamGroups(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("UpstreamGroup %s.%s query error: %w", apexName, canary.Namespace, err)
	}

	upstreamGroup.Spec = gloov1.UpstreamGroupSpec{
		Destinations: []gloov1.WeightedDestination{
			{
				Destination: gloov1.Destination{
					Upstream: gloov1.ResourceRef{
						Name:      primaryName,
						Namespace: gr.upstreamDiscoveryNs,
					},
				},
				Weight: uint32(primaryWeight),
			},
			{
				Destination: gloov1.Destination{
					Upstream: gloov1.ResourceRef{
						Name:      canaryName,
						Namespace: gr.upstreamDiscoveryNs,
					},
				},
				Weight: uint32(canaryWeight),
			},
		},
	}

	_, err = gr.glooClient.GlooV1().UpstreamGroups(canary.Namespace).Update(context.TODO(), upstreamGroup, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("UpstreamGroup %s.%s update error: %w", apexName, canary.Namespace, err)
	}
	return nil
}

func (gr *GlooRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
