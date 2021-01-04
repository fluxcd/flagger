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

// GlooRouter is managing Gloo route tables
type GlooRouter struct {
	kubeClient          kubernetes.Interface
	glooClient          clientset.Interface
	flaggerClient       clientset.Interface
	logger              *zap.SugaredLogger
	upstreamDiscoveryNs string
}

// Reconcile creates or updates the Gloo Edge route table
func (gr *GlooRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, _, _ := canary.GetServiceNames()
	canaryName := fmt.Sprintf("%s-%s-canary-%v", canary.Namespace, apexName, canary.Spec.Service.Port)
	primaryName := fmt.Sprintf("%s-%s-primary-%v", canary.Namespace, apexName, canary.Spec.Service.Port)

	newSpec := gloov1.RouteTableSpec{
		Routes: []gloov1.Route{
			{
				InheritablePathMatchers: true,
				Matchers:                getMatchers(canary),
				Action: gloov1.RouteAction{
					Destination: gloov1.MultiDestination{
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
					},
				},
			},
		},
	}

	routeTable, err := gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if errors.IsNotFound(err) {

		routeTable = &gloov1.RouteTable{
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

		_, err = gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Create(context.TODO(), routeTable, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("RouteTable %s.%s create error: %w", apexName, canary.Namespace, err)
		}
		gr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("RouteTable %s.%s created", routeTable.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("RouteTable %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	// update routeTable but keep the original destination weights
	if routeTable != nil {
		if diff := cmp.Diff(
			newSpec,
			routeTable.Spec,
			cmpopts.IgnoreFields(gloov1.WeightedDestination{}, "Weight"),
		); diff != "" {
			clone := routeTable.DeepCopy()
			clone.Spec = newSpec

			_, err = gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("RouteTable %s.%s update error: %w", apexName, canary.Namespace, err)
			}
			gr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("RouteTable %s.%s updated", routeTable.GetName(), canary.Namespace)
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

	routeTable, err := gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("RouteTable %s.%s get query error: %w", apexName, canary.Namespace, err)
		return
	}

	if len(routeTable.Spec.Routes[0].Action.Destination.Destinations) < 2 {
		err = fmt.Errorf("RouteTable %s.%s destinations not found", apexName, canary.Namespace)
		return
	}

	for _, dst := range routeTable.Spec.Routes[0].Action.Destination.Destinations {
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

	routeTable, err := gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("RouteTable %s.%s query error: %w", apexName, canary.Namespace, err)
	}

	routeTable.Spec = gloov1.RouteTableSpec{
		Routes: []gloov1.Route{
			{
				InheritablePathMatchers: true,
				Matchers:                getMatchers(canary),
				Action: gloov1.RouteAction{
					Destination: gloov1.MultiDestination{
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
					},
				},
			},
		},
	}

	_, err = gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Update(context.TODO(), routeTable, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("RouteTable %s.%s update error: %w", apexName, canary.Namespace, err)
	}
	return nil
}

func (gr *GlooRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}

func getMatchers(canary *flaggerv1.Canary) []gloov1.Matcher {

	headerMatchers := getHeaderMatchers(canary)
	methods := getMethods(canary)

	if len(headerMatchers) == 0 && len(methods) == 0 {
		return nil
	}

	return []gloov1.Matcher{
		{
			Headers: headerMatchers,
			Methods: methods,
		},
	}
}

func getHeaderMatchers(canary *flaggerv1.Canary) []gloov1.HeaderMatcher {
	var headerMatchers []gloov1.HeaderMatcher
	for _, match := range canary.GetAnalysis().Match {
		for s, stringMatch := range match.Headers {
			h := gloov1.HeaderMatcher{
				Name:  s,
				Value: stringMatch.Exact,
			}
			if stringMatch.Regex != "" {
				h = gloov1.HeaderMatcher{
					Name:  s,
					Value: stringMatch.Regex,
					Regex: true,
				}
			}
			headerMatchers = append(headerMatchers, h)
		}
	}
	return headerMatchers
}

func getMethods(canary *flaggerv1.Canary) []string {
	var methods []string
	for _, match := range canary.GetAnalysis().Match {
		if stringMatch := match.Method; stringMatch != nil {
			methods = append(methods, stringMatch.Exact)
		}
	}
	return methods
}
