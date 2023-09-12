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
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/fluxcd/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// IstioRouter is managing Istio virtual services
type IstioRouter struct {
	kubeClient    kubernetes.Interface
	istioClient   clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	setOwnerRefs  bool
}

const cookieHeader = "Cookie"
const setCookieHeader = "Set-Cookie"
const stickyRouteName = "sticky-route"
const maxAgeAttr = "Max-Age"

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// Reconcile creates or updates the Istio virtual service and destination rules
func (ir *IstioRouter) Reconcile(canary *flaggerv1.Canary) error {
	_, primaryName, canaryName := canary.GetServiceNames()

	if err := ir.reconcileDestinationRule(canary, canaryName); err != nil {
		return fmt.Errorf("reconcileDestinationRule failed: %w", err)
	}

	if err := ir.reconcileDestinationRule(canary, primaryName); err != nil {
		return fmt.Errorf("reconcileDestinationRule failed: %w", err)
	}

	if err := ir.reconcileVirtualService(canary); err != nil {
		return fmt.Errorf("reconcileVirtualService failed: %w", err)
	}
	return nil
}

func (ir *IstioRouter) reconcileDestinationRule(canary *flaggerv1.Canary, name string) error {
	newSpec := istiov1alpha3.DestinationRuleSpec{
		Host:          name,
		TrafficPolicy: canary.Spec.Service.TrafficPolicy,
	}

	destinationRule, err := ir.istioClient.NetworkingV1alpha3().DestinationRules(canary.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
	// insert
	if errors.IsNotFound(err) {
		destinationRule = &istiov1alpha3.DestinationRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: canary.Namespace,
			},
			Spec: newSpec,
		}
		if ir.setOwnerRefs {
			destinationRule.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
		}
		_, err = ir.istioClient.NetworkingV1alpha3().DestinationRules(canary.Namespace).Create(context.TODO(), destinationRule, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("DestinationRule %s.%s create error: %w", name, canary.Namespace, err)
		}
		ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("DestinationRule %s.%s created", destinationRule.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("DestinationRule %s.%s get query error: %w", name, canary.Namespace, err)
	}

	// update
	if destinationRule != nil {
		if diff := cmp.Diff(newSpec, destinationRule.Spec); diff != "" {
			clone := destinationRule.DeepCopy()
			clone.Spec = newSpec
			_, err = ir.istioClient.NetworkingV1alpha3().DestinationRules(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("DestinationRule %s.%s update error: %w", name, canary.Namespace, err)
			}
			ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("DestinationRule %s.%s updated", destinationRule.GetName(), canary.Namespace)
		}
	}

	return nil
}

func (ir *IstioRouter) reconcileVirtualService(canary *flaggerv1.Canary) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	if canary.Spec.Service.Delegation {
		if len(canary.Spec.Service.Hosts) > 0 || len(canary.Spec.Service.Gateways) > 0 {
			// delegate VirtualService cannot have hosts and gateways.
			return fmt.Errorf("VirtualService %s.%s cannot have hosts and gateways when delegation enabled", apexName, canary.Namespace)
		}
	}

	// set hosts and add the ClusterIP service host if it doesn't exists
	hosts := canary.Spec.Service.Hosts
	var hasServiceHost bool
	for _, h := range hosts {
		if h == apexName || h == "*" {
			hasServiceHost = true
			break
		}
	}
	if !hasServiceHost {
		hosts = append(hosts, apexName)
	}

	// set gateways and add the mesh gateway if it doesn't exists
	gateways := canary.Spec.Service.Gateways
	var hasMeshGateway bool
	for _, g := range gateways {
		if g == "mesh" {
			hasMeshGateway = true
			break
		}
	}

	// set default mesh gateway if no gateway is specified
	if !hasMeshGateway && len(canary.Spec.Service.Gateways) == 0 {
		gateways = append(gateways, "mesh")
	}

	// create destinations with primary weight 100% and canary weight 0%
	canaryRoute := []istiov1alpha3.HTTPRouteDestination{
		makeDestination(canary, primaryName, 100),
		makeDestination(canary, canaryName, 0),
	}

	if canary.Spec.Service.Delegation {
		// delegate VirtualService requires the hosts and gateway empty.
		hosts = []string{}
		gateways = []string{}
	}

	newSpec := istiov1alpha3.VirtualServiceSpec{
		Hosts:    hosts,
		Gateways: gateways,
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match:      canary.Spec.Service.Match,
				Rewrite:    canary.Spec.Service.GetIstioRewrite(),
				Timeout:    canary.Spec.Service.Timeout,
				Retries:    canary.Spec.Service.Retries,
				CorsPolicy: canary.Spec.Service.CorsPolicy,
				Headers:    canary.Spec.Service.Headers,
				Route:      canaryRoute,
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

	if len(canary.GetAnalysis().Match) > 0 {
		canaryMatch := mergeMatchConditions(canary.GetAnalysis().Match, canary.Spec.Service.Match)
		newSpec.Http = []istiov1alpha3.HTTPRoute{
			{
				Match:      canaryMatch,
				Rewrite:    canary.Spec.Service.GetIstioRewrite(),
				Timeout:    canary.Spec.Service.Timeout,
				Retries:    canary.Spec.Service.Retries,
				CorsPolicy: canary.Spec.Service.CorsPolicy,
				Headers:    canary.Spec.Service.Headers,
				Route:      canaryRoute,
			},
			{
				Match:      canary.Spec.Service.Match,
				Rewrite:    canary.Spec.Service.GetIstioRewrite(),
				Timeout:    canary.Spec.Service.Timeout,
				Retries:    canary.Spec.Service.Retries,
				CorsPolicy: canary.Spec.Service.CorsPolicy,
				Headers:    canary.Spec.Service.Headers,
				Route: []istiov1alpha3.HTTPRouteDestination{
					makeDestination(canary, primaryName, 100),
				},
			},
		}
	}

	virtualService, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	// insert
	if errors.IsNotFound(err) {
		virtualService = &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        apexName,
				Namespace:   canary.Namespace,
				Labels:      newMetadata.Labels,
				Annotations: newMetadata.Annotations,
			},
			Spec: newSpec,
		}
		if ir.setOwnerRefs {
			virtualService.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
		}
		_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Create(context.TODO(), virtualService, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("VirtualService %s.%s create error: %w", apexName, canary.Namespace, err)
		}
		ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualService %s.%s created", virtualService.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("VirtualService %s.%s get query error %v", apexName, canary.Namespace, err)
	}

	if canary.Spec.Service.Delegation {
		// delegate VirtualService requires the hosts and gateway empty.
		virtualService.Spec.Gateways = []string{}
		virtualService.Spec.Hosts = []string{}
	}

	ignoreCmpOptions := []cmp.Option{
		cmpopts.IgnoreFields(istiov1alpha3.HTTPRouteDestination{}, "Weight"),
		cmpopts.IgnoreFields(istiov1alpha3.HTTPRoute{}, "Mirror", "MirrorPercentage"),
	}
	if canary.Spec.Analysis.SessionAffinity != nil {
		// We ignore this route as this does not do weighted routing and is handled exclusively
		// by SetRoutes().
		ignoreSlice := cmpopts.IgnoreSliceElements(func(t istiov1alpha3.HTTPRoute) bool {
			if t.Name == stickyRouteName {
				return true
			}
			return false
		})
		ignoreCmpOptions = append(ignoreCmpOptions, ignoreSlice)
		ignoreCmpOptions = append(ignoreCmpOptions, cmpopts.IgnoreFields(istiov1alpha3.HTTPRouteDestination{}, "Headers"))
	}
	if v, ok := virtualService.Annotations[kubectlAnnotation]; ok {
		newMetadata.Annotations[kubectlAnnotation] = v
	}
	if v, ok := virtualService.Annotations[configAnnotation]; ok {
		newMetadata.Annotations[configAnnotation] = v
	}
	// update service but keep the original destination weights and mirror
	if virtualService != nil {
		specDiff := cmp.Diff(
			newSpec,
			virtualService.Spec,
			ignoreCmpOptions...,
		)
		labelsDiff := cmp.Diff(newMetadata.Labels, virtualService.Labels, cmpopts.EquateEmpty())
		annotationsDiff := cmp.Diff(newMetadata.Annotations, virtualService.Annotations, cmpopts.EquateEmpty())
		if specDiff != "" || labelsDiff != "" || annotationsDiff != "" {
			vtClone := virtualService.DeepCopy()
			vtClone.Spec = newSpec
			vtClone.ObjectMeta.Annotations = newMetadata.Annotations
			vtClone.ObjectMeta.Labels = newMetadata.Labels

			//If annotation kubectl.kubernetes.io/last-applied-configuration is present no need to duplicate
			//serialization.  If not present store the serialized object in annotation
			//flagger.kubernetes.app/original-configuration
			if _, ok := vtClone.Annotations[kubectlAnnotation]; !ok && specDiff != "" {
				b, err := json.Marshal(virtualService.Spec)
				if err != nil {
					ir.logger.Warnf("Unable to marshal VS %s for orig-configuration annotation", virtualService.Name)
				}

				if vtClone.ObjectMeta.Annotations == nil {
					vtClone.ObjectMeta.Annotations = make(map[string]string)
				} else {
					vtClone.ObjectMeta.Annotations = filterMetadata(vtClone.ObjectMeta.Annotations)
				}

				vtClone.ObjectMeta.Annotations[configAnnotation] = string(b)
			}

			_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Update(context.TODO(), vtClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("VirtualService %s.%s update error: %w", apexName, canary.Namespace, err)
			}
			ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("VirtualService %s.%s updated", virtualService.GetName(), canary.Namespace)
		}
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (ir *IstioRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	vs := &istiov1alpha3.VirtualService{}
	vs, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("VirtualService %s.%s get query error %v", apexName, canary.Namespace, err)
		return
	}

	var httpRoute istiov1alpha3.HTTPRoute
	for _, http := range vs.Spec.Http {
		for _, r := range http.Route {
			if r.Destination.Host == canaryName {
				httpRoute = http
				break
			}
		}
	}

	for _, route := range httpRoute.Route {
		if route.Destination.Host == primaryName {
			primaryWeight = route.Weight
		}
		if route.Destination.Host == canaryName {
			canaryWeight = route.Weight
		}
	}
	if httpRoute.Mirror != nil && httpRoute.Mirror.Host != "" {
		mirrored = true
	}

	if canary.Spec.Analysis.SessionAffinity != nil {
		for _, http := range vs.Spec.Http {
			for _, routeDest := range http.Route {
				// we are interested in the route that sets the cookie as that's the route
				// that does weighted routing.
				if routeDest.Headers != nil {
					if routeDest.Destination.Host == primaryName {
						primaryWeight = routeDest.Weight
					}
					if routeDest.Destination.Host == canaryName {
						canaryWeight = routeDest.Weight
					}
				}
			}
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("VirtualService %s.%s does not contain routes for %s-primary and %s-canary",
			apexName, canary.Namespace, apexName, apexName)
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (ir *IstioRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	vs, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s get query error %v", apexName, canary.Namespace, err)
	}

	vsCopy := vs.DeepCopy()

	// weighted routing (progressive canary)
	weightedRoute := istiov1alpha3.HTTPRoute{
		Match:      canary.Spec.Service.Match,
		Rewrite:    canary.Spec.Service.GetIstioRewrite(),
		Timeout:    canary.Spec.Service.Timeout,
		Retries:    canary.Spec.Service.Retries,
		CorsPolicy: canary.Spec.Service.CorsPolicy,
		Headers:    canary.Spec.Service.Headers,
		Route: []istiov1alpha3.HTTPRouteDestination{
			makeDestination(canary, primaryName, primaryWeight),
			makeDestination(canary, canaryName, canaryWeight),
		},
	}
	vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
		weightedRoute,
	}

	if canary.Spec.Analysis.SessionAffinity != nil {
		// If a canary run is active, we want all responses corresponding to requests hitting the canary deployment
		// (due to weighted routing) to include a `Set-Cookie` header. All requests that have the `Cookie` header
		// and match the value of the `Set-Cookie` header will be routed to the canary deployment.
		stickyRoute := weightedRoute
		stickyRoute.Name = stickyRouteName
		if canaryWeight != 0 {
			if canary.Status.SessionAffinityCookie == "" {
				canary.Status.SessionAffinityCookie = fmt.Sprintf("%s=%s", canary.Spec.Analysis.SessionAffinity.CookieName, randSeq())
			}

			for i, routeDest := range weightedRoute.Route {
				if routeDest.Destination.Host == canaryName {
					if routeDest.Headers == nil {
						routeDest.Headers = &istiov1alpha3.Headers{
							Response: &istiov1alpha3.HeaderOperations{},
						}
					}
					routeDest.Headers.Response.Add = map[string]string{
						setCookieHeader: fmt.Sprintf("%s; %s=%d", canary.Status.SessionAffinityCookie, maxAgeAttr,
							canary.Spec.Analysis.SessionAffinity.GetMaxAge(),
						),
					}
				}
				weightedRoute.Route[i] = routeDest
			}

			cookieKeyAndVal := strings.Split(canary.Status.SessionAffinityCookie, "=")
			cookieMatch := istiov1alpha3.HTTPMatchRequest{
				Headers: map[string]istiov1alpha1.StringMatch{
					cookieHeader: {
						Regex: fmt.Sprintf(".*%s.*%s.*", cookieKeyAndVal[0], cookieKeyAndVal[1]),
					},
				},
			}
			canaryMatch := mergeMatchConditions([]istiov1alpha3.HTTPMatchRequest{cookieMatch}, canary.Spec.Service.Match)
			stickyRoute.Match = canaryMatch
			stickyRoute.Route = []istiov1alpha3.HTTPRouteDestination{
				makeDestination(canary, primaryName, 0),
				makeDestination(canary, canaryName, 100),
			}
		} else {
			// If canary weight is 0 and SessionAffinityCookie is non-blank, then it belongs to a previous canary run.
			if canary.Status.SessionAffinityCookie != "" {
				canary.Status.PreviousSessionAffinityCookie = canary.Status.SessionAffinityCookie
			}
			previousCookie := canary.Status.PreviousSessionAffinityCookie

			// Match against the previous session cookie and delete that cookie
			if previousCookie != "" {
				cookieKeyAndVal := strings.Split(previousCookie, "=")
				cookieMatch := istiov1alpha3.HTTPMatchRequest{
					Headers: map[string]istiov1alpha1.StringMatch{
						cookieHeader: {
							Regex: fmt.Sprintf(".*%s.*%s.*", cookieKeyAndVal[0], cookieKeyAndVal[1]),
						},
					},
				}
				canaryMatch := mergeMatchConditions([]istiov1alpha3.HTTPMatchRequest{cookieMatch}, canary.Spec.Service.Match)
				stickyRoute.Match = canaryMatch

				if stickyRoute.Headers == nil {
					stickyRoute.Headers = &istiov1alpha3.Headers{
						Response: &istiov1alpha3.HeaderOperations{
							Add: map[string]string{},
						},
					}
				} else if stickyRoute.Headers.Response == nil {
					stickyRoute.Headers.Response = &istiov1alpha3.HeaderOperations{
						Add: map[string]string{},
					}
				} else if stickyRoute.Headers.Response.Add == nil {
					stickyRoute.Headers.Response.Add = map[string]string{}
				}
				stickyRoute.Headers.Response.Add[setCookieHeader] = fmt.Sprintf("%s; %s=%d", previousCookie, maxAgeAttr, -1)
			}

			canary.Status.SessionAffinityCookie = ""
		}
		vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
			stickyRoute, weightedRoute,
		}
	}

	if mirrored {
		vsCopy.Spec.Http[0].Mirror = &istiov1alpha3.Destination{
			Host: canaryName,
		}

		if mw := canary.GetAnalysis().MirrorWeight; mw > 0 {
			vsCopy.Spec.Http[0].MirrorPercentage = &istiov1alpha3.Percent{Value: float64(mw)}
		}
	}

	// fix routing (A/B testing)
	if len(canary.GetAnalysis().Match) > 0 {
		// merge the common routes with the canary ones
		canaryMatch := mergeMatchConditions(canary.GetAnalysis().Match, canary.Spec.Service.Match)
		vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
			{
				Match:      canaryMatch,
				Rewrite:    canary.Spec.Service.GetIstioRewrite(),
				Timeout:    canary.Spec.Service.Timeout,
				Retries:    canary.Spec.Service.Retries,
				CorsPolicy: canary.Spec.Service.CorsPolicy,
				Headers:    canary.Spec.Service.Headers,
				Route: []istiov1alpha3.HTTPRouteDestination{
					makeDestination(canary, primaryName, primaryWeight),
					makeDestination(canary, canaryName, canaryWeight),
				},
			},
			{
				Match:      canary.Spec.Service.Match,
				Rewrite:    canary.Spec.Service.GetIstioRewrite(),
				Timeout:    canary.Spec.Service.Timeout,
				Retries:    canary.Spec.Service.Retries,
				CorsPolicy: canary.Spec.Service.CorsPolicy,
				Headers:    canary.Spec.Service.Headers,
				Route: []istiov1alpha3.HTTPRouteDestination{
					makeDestination(canary, primaryName, primaryWeight),
				},
			},
		}
	}

	vs, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Update(context.TODO(), vsCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s update failed: %w", apexName, canary.Namespace, err)
	}
	return nil
}

func (ir *IstioRouter) Finalize(canary *flaggerv1.Canary) error {
	// Need to see if I can get the annotation orig-configuration
	apexName, _, _ := canary.GetServiceNames()

	vs, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	var storedSpec istiov1alpha3.VirtualServiceSpec
	if a, ok := vs.ObjectMeta.Annotations[kubectlAnnotation]; ok {
		var storedVS istiov1alpha3.VirtualService
		if err := json.Unmarshal([]byte(a), &storedVS); err != nil {
			return fmt.Errorf("VirtualService %s.%s failed to unMarshal annotation %s",
				apexName, canary.Namespace, kubectlAnnotation)
		}
		storedSpec = storedVS.Spec
	} else if a, ok := vs.ObjectMeta.Annotations[configAnnotation]; ok {
		if err := json.Unmarshal([]byte(a), &storedSpec); err != nil {
			return fmt.Errorf("VirtualService %s.%s failed to unMarshal annotation %s",
				apexName, canary.Namespace, configAnnotation)
		}
	} else {
		ir.logger.Warnf("VirtualService %s.%s original configuration not found, unable to revert", apexName, canary.Namespace)
		return nil
	}

	clone := vs.DeepCopy()
	clone.Spec = storedSpec

	_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s update error: %w", apexName, canary.Namespace, err)
	}
	return nil
}

// mergeMatchConditions appends the URI match rules to canary conditions
func mergeMatchConditions(canary, defaults []istiov1alpha3.HTTPMatchRequest) []istiov1alpha3.HTTPMatchRequest {
	if len(defaults) == 0 {
		return canary
	}

	merged := make([]istiov1alpha3.HTTPMatchRequest, len(canary)*len(defaults))
	num := 0
	for _, c := range canary {
		for _, d := range defaults {
			merged[num] = *d.DeepCopy()
			if c.Headers != nil {
				merged[num].Headers = c.Headers
			}
			if c.SourceLabels != nil {
				merged[num].SourceLabels = c.SourceLabels
			}
			num++
		}
	}

	return merged
}

// makeDestination returns a an destination weight for the specified host
func makeDestination(canary *flaggerv1.Canary, host string, weight int) istiov1alpha3.HTTPRouteDestination {
	dest := istiov1alpha3.HTTPRouteDestination{
		Destination: istiov1alpha3.Destination{
			Host: host,
		},
		Weight: weight,
	}

	// set destination port when an ingress gateway is specified
	if canary.Spec.Service.PortDiscovery &&
		(len(canary.Spec.Service.Gateways) > 0 &&
			canary.Spec.Service.Gateways[0] != "mesh" || canary.Spec.Service.Delegation) {
		dest = istiov1alpha3.HTTPRouteDestination{
			Destination: istiov1alpha3.Destination{
				Host: host,
				Port: &istiov1alpha3.PortSelector{
					Number: uint32(canary.Spec.Service.Port),
				},
			},
			Weight: weight,
		}
	}

	return dest
}

func randSeq() string {
	rand.Seed(time.Now().UnixNano())

	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
