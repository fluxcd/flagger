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

	corev1 "k8s.io/api/core/v1"

	gatewayv1 "github.com/fluxcd/flagger/pkg/apis/gloo/gateway/v1"
	gloov1 "github.com/fluxcd/flagger/pkg/apis/gloo/gloo/v1"
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
	kubeClient         kubernetes.Interface
	glooClient         clientset.Interface
	flaggerClient      clientset.Interface
	logger             *zap.SugaredLogger
	includeLabelPrefix []string
	setOwnerRefs       bool
}

// Reconcile creates or updates the Gloo Edge route table
func (gr *GlooRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, _, _ := canary.GetServiceNames()
	canaryUpstreamName := fmt.Sprintf("%s-%s-canaryupstream-%v", canary.Namespace, apexName, canary.Spec.Service.Port)
	primaryUpstreamName := fmt.Sprintf("%s-%s-primaryupstream-%v", canary.Namespace, apexName, canary.Spec.Service.Port)

	// Create upstreams for the canary/primary services created by flagger.
	// Previously, we relied on gloo discovery to automaticallycreate these upstreams, but this would no longer work if
	// discovery was turned off.
	// KubeServiceDestinations can be disabled in gloo configuration, so we don't use those either.
	err := gr.createFlaggerUpstream(canary, primaryUpstreamName, false)
	if err != nil {
		return fmt.Errorf("error creating flagger primary upstream: %w", err)
	}
	err = gr.createFlaggerUpstream(canary, canaryUpstreamName, true)
	if err != nil {
		return fmt.Errorf("error creating flagger canary upstream: %w", err)
	}

	newSpec := gatewayv1.RouteTableSpec{
		Routes: []gatewayv1.Route{
			{
				InheritablePathMatchers: true,
				Matchers:                getMatchers(canary),
				Action: gatewayv1.RouteAction{
					Destination: gatewayv1.MultiDestination{
						Destinations: []gatewayv1.WeightedDestination{
							{
								Destination: gatewayv1.Destination{
									Upstream: gatewayv1.ResourceRef{
										Name:      primaryUpstreamName,
										Namespace: canary.Namespace,
									},
								},
								Weight: 100,
							},
							{
								Destination: gatewayv1.Destination{
									Upstream: gatewayv1.ResourceRef{
										Name:      canaryUpstreamName,
										Namespace: canary.Namespace,
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

	newMetadata := canary.Spec.Service.Apex
	if newMetadata == nil {
		newMetadata = &flaggerv1.CustomMetadata{}
	}
	if newMetadata.Labels == nil {
		newMetadata.Labels = make(map[string]string)
	}
	if newMetadata.Annotations == nil {
		newMetadata.Annotations = make(map[string]string)
	}
	newMetadata.Annotations = filterMetadata(newMetadata.Annotations)

	routeTable, err := gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		routeTable = &gatewayv1.RouteTable{
			ObjectMeta: metav1.ObjectMeta{
				Name:        apexName,
				Namespace:   canary.Namespace,
				Labels:      newMetadata.Labels,
				Annotations: newMetadata.Annotations,
			},
			Spec: newSpec,
		}
		if gr.setOwnerRefs {
			routeTable.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
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
		specDiff := cmp.Diff(
			newSpec,
			routeTable.Spec,
			cmpopts.IgnoreFields(gatewayv1.WeightedDestination{}, "Weight"),
		)
		labelsDiff := cmp.Diff(newMetadata.Labels, routeTable.Labels, cmpopts.EquateEmpty())
		annotationsDiff := cmp.Diff(newMetadata.Annotations, routeTable.Annotations, cmpopts.EquateEmpty())
		if specDiff != "" || labelsDiff != "" || annotationsDiff != "" {
			clone := routeTable.DeepCopy()
			clone.Spec = newSpec
			clone.ObjectMeta.Annotations = newMetadata.Annotations
			clone.ObjectMeta.Labels = newMetadata.Labels

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
	apexName, _, _ := canary.GetServiceNames()
	primaryUpstreamName := fmt.Sprintf("%s-%s-primaryupstream-%v", canary.Namespace, apexName, canary.Spec.Service.Port)
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
		if dst.Destination.Upstream.Name == primaryUpstreamName {
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
	canaryName := fmt.Sprintf("%s-%s-canaryupstream-%v", canary.Namespace, apexName, canary.Spec.Service.Port)
	primaryName := fmt.Sprintf("%s-%s-primaryupstream-%v", canary.Namespace, apexName, canary.Spec.Service.Port)

	if primaryWeight == 0 && canaryWeight == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", apexName, canary.Namespace)
	}

	routeTable, err := gr.glooClient.GatewayV1().RouteTables(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("RouteTable %s.%s query error: %w", apexName, canary.Namespace, err)
	}

	routeTable.Spec = gatewayv1.RouteTableSpec{
		Routes: []gatewayv1.Route{
			{
				InheritablePathMatchers: true,
				Matchers:                getMatchers(canary),
				Action: gatewayv1.RouteAction{
					Destination: gatewayv1.MultiDestination{
						Destinations: []gatewayv1.WeightedDestination{
							{
								Destination: gatewayv1.Destination{
									Upstream: gatewayv1.ResourceRef{
										Name:      primaryName,
										Namespace: canary.Namespace,
									},
								},
								Weight: uint32(primaryWeight),
							},
							{
								Destination: gatewayv1.Destination{
									Upstream: gatewayv1.ResourceRef{
										Name:      canaryName,
										Namespace: canary.Namespace,
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

func (gr *GlooRouter) createFlaggerUpstream(canary *flaggerv1.Canary, upstreamName string, isCanary bool) error {
	_, primaryName, canaryName := canary.GetServiceNames()
	upstreamClient := gr.glooClient.GlooV1().Upstreams(canary.Namespace)
	svcName := primaryName
	if isCanary {
		svcName = canaryName
	}
	svc, err := gr.kubeClient.CoreV1().Services(canary.Namespace).Get(context.TODO(), svcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", svcName, canary.Namespace, err)
	}
	_, err = upstreamClient.Get(context.TODO(), upstreamName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		glooUpstreamWithConfig, err := gr.getGlooConfigUpstream(canary)
		if err != nil {
			return err
		}
		canaryUs := gr.getGlooUpstreamKubeService(canary, svc, upstreamName, glooUpstreamWithConfig)
		_, err = gr.glooClient.GlooV1().Upstreams(canary.Namespace).Create(context.TODO(), canaryUs, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("upstream %s.%s create query error: %w", upstreamName, canary.Namespace, err)
		}
	} else if err != nil {
		return fmt.Errorf("upstream %s.%s get query error: %w", upstreamName, canary.Namespace, err)
	}
	return nil
}

func (gr *GlooRouter) getGlooUpstreamKubeService(canary *flaggerv1.Canary, svc *corev1.Service, upstreamName string, glooUpstreamWithConfig *gloov1.Upstream) *gloov1.Upstream {

	upstreamSpec := gloov1.UpstreamSpec{}
	if glooUpstreamWithConfig != nil {
		configSpec := glooUpstreamWithConfig.Spec
		upstreamSpec = gloov1.UpstreamSpec{
			Labels:                      configSpec.Labels,
			SslConfig:                   configSpec.SslConfig,
			CircuitBreakers:             configSpec.CircuitBreakers,
			ConnectionConfig:            configSpec.ConnectionConfig,
			LoadBalancerConfig:          configSpec.LoadBalancerConfig,
			UseHttp2:                    configSpec.UseHttp2,
			InitialStreamWindowSize:     configSpec.InitialStreamWindowSize,
			InitialConnectionWindowSize: configSpec.InitialConnectionWindowSize,
			HttpProxyHostname:           configSpec.HttpProxyHostname,
		}

	}
	upstreamSpec.Kube = &gloov1.KubeUpstream{
		ServiceName:      svc.GetName(),
		ServiceNamespace: canary.Namespace,
		ServicePort:      canary.Spec.Service.Port,
		Selector:         svc.Spec.Selector,
	}

	upstreamLabels := includeLabelsByPrefix(upstreamSpec.Labels, gr.includeLabelPrefix)

	upstream := &gloov1.Upstream{
		ObjectMeta: metav1.ObjectMeta{
			Name:      upstreamName,
			Namespace: canary.Namespace,
			Labels:    upstreamLabels,
		},
		Spec: upstreamSpec,
	}
	if gr.setOwnerRefs {
		upstream.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(canary, schema.GroupVersionKind{
				Group:   flaggerv1.SchemeGroupVersion.Group,
				Version: flaggerv1.SchemeGroupVersion.Version,
				Kind:    flaggerv1.CanaryKind,
			}),
		}
	}
	return upstream
}

func (gr *GlooRouter) getGlooConfigUpstream(canary *flaggerv1.Canary) (*gloov1.Upstream, error) {
	configUpstreamRef := canary.Spec.UpstreamRef
	if configUpstreamRef == nil {
		return nil, nil
	}
	configUpstream, err := gr.glooClient.GlooV1().Upstreams(configUpstreamRef.Namespace).Get(context.TODO(), configUpstreamRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("config upstream %s.%s get query error: %w", configUpstreamRef.Name, canary.Namespace, err)
	}
	return configUpstream, nil
}

func getMatchers(canary *flaggerv1.Canary) []gatewayv1.Matcher {

	headerMatchers := getHeaderMatchers(canary)
	methods := getMethods(canary)

	if len(headerMatchers) == 0 && len(methods) == 0 {
		return nil
	}

	return []gatewayv1.Matcher{
		{
			Headers: headerMatchers,
			Methods: methods,
		},
	}
}

func getHeaderMatchers(canary *flaggerv1.Canary) []gatewayv1.HeaderMatcher {
	var headerMatchers []gatewayv1.HeaderMatcher
	for _, match := range canary.GetAnalysis().Match {
		for s, stringMatch := range match.Headers {
			h := gatewayv1.HeaderMatcher{
				Name:  s,
				Value: stringMatch.Exact,
			}
			if stringMatch.Regex != "" {
				h = gatewayv1.HeaderMatcher{
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
