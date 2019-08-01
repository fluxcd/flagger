package router

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// IstioRouter is managing Istio virtual services
type IstioRouter struct {
	kubeClient    kubernetes.Interface
	istioClient   clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Reconcile creates or updates the Istio virtual service and destination rules
func (ir *IstioRouter) Reconcile(canary *flaggerv1.Canary) error {
	canaryName := fmt.Sprintf("%s-canary", canary.Spec.TargetRef.Name)
	primaryName := fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)

	err := ir.reconcileDestinationRule(canary, canaryName)
	if err != nil {
		return err
	}

	err = ir.reconcileDestinationRule(canary, primaryName)
	if err != nil {
		return err
	}

	err = ir.reconcileVirtualService(canary)
	if err != nil {
		return err
	}

	return nil
}

func (ir *IstioRouter) reconcileDestinationRule(canary *flaggerv1.Canary, name string) error {
	newSpec := istiov1alpha3.DestinationRuleSpec{
		Host:          name,
		TrafficPolicy: canary.Spec.Service.TrafficPolicy,
	}

	destinationRule, err := ir.istioClient.NetworkingV1alpha3().DestinationRules(canary.Namespace).Get(name, metav1.GetOptions{})
	// insert
	if errors.IsNotFound(err) {
		destinationRule = &istiov1alpha3.DestinationRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
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
		_, err = ir.istioClient.NetworkingV1alpha3().DestinationRules(canary.Namespace).Create(destinationRule)
		if err != nil {
			return fmt.Errorf("DestinationRule %s.%s create error %v", name, canary.Namespace, err)
		}
		ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("DestinationRule %s.%s created", destinationRule.GetName(), canary.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("DestinationRule %s.%s query error %v", name, canary.Namespace, err)
	}

	// update
	if destinationRule != nil {
		if diff := cmp.Diff(newSpec, destinationRule.Spec); diff != "" {
			clone := destinationRule.DeepCopy()
			clone.Spec = newSpec
			_, err = ir.istioClient.NetworkingV1alpha3().DestinationRules(canary.Namespace).Update(clone)
			if err != nil {
				return fmt.Errorf("DestinationRule %s.%s update error %v", name, canary.Namespace, err)
			}
			ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("DestinationRule %s.%s updated", destinationRule.GetName(), canary.Namespace)
		}
	}

	return nil
}

func (ir *IstioRouter) reconcileVirtualService(canary *flaggerv1.Canary) error {
	targetName := canary.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	// set hosts and add the ClusterIP service host if it doesn't exists
	hosts := canary.Spec.Service.Hosts
	var hasServiceHost bool
	for _, h := range hosts {
		if h == targetName || h == "*" {
			hasServiceHost = true
			break
		}
	}
	if !hasServiceHost {
		hosts = append(hosts, targetName)
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
	canaryRoute := []istiov1alpha3.DestinationWeight{
		makeDestination(canary, primaryName, 100),
		makeDestination(canary, canaryName, 0),
	}

	newSpec := istiov1alpha3.VirtualServiceSpec{
		Hosts:    hosts,
		Gateways: gateways,
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match:         canary.Spec.Service.Match,
				Rewrite:       canary.Spec.Service.Rewrite,
				Timeout:       canary.Spec.Service.Timeout,
				Retries:       canary.Spec.Service.Retries,
				CorsPolicy:    canary.Spec.Service.CorsPolicy,
				AppendHeaders: addHeaders(canary),
				Route:         canaryRoute,
			},
		},
	}

	if len(canary.Spec.CanaryAnalysis.Match) > 0 {
		canaryMatch := mergeMatchConditions(canary.Spec.CanaryAnalysis.Match, canary.Spec.Service.Match)
		newSpec.Http = []istiov1alpha3.HTTPRoute{
			{
				Match:         canaryMatch,
				Rewrite:       canary.Spec.Service.Rewrite,
				Timeout:       canary.Spec.Service.Timeout,
				Retries:       canary.Spec.Service.Retries,
				CorsPolicy:    canary.Spec.Service.CorsPolicy,
				AppendHeaders: addHeaders(canary),
				Route:         canaryRoute,
			},
			{
				Match:         canary.Spec.Service.Match,
				Rewrite:       canary.Spec.Service.Rewrite,
				Timeout:       canary.Spec.Service.Timeout,
				Retries:       canary.Spec.Service.Retries,
				CorsPolicy:    canary.Spec.Service.CorsPolicy,
				AppendHeaders: addHeaders(canary),
				Route: []istiov1alpha3.DestinationWeight{
					makeDestination(canary, primaryName, 100),
				},
			},
		}
	}

	virtualService, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(targetName, metav1.GetOptions{})
	// insert
	if errors.IsNotFound(err) {
		virtualService = &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
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
		_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Create(virtualService)
		if err != nil {
			return fmt.Errorf("VirtualService %s.%s create error %v", targetName, canary.Namespace, err)
		}
		ir.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualService %s.%s created", virtualService.GetName(), canary.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("VirtualService %s.%s query error %v", targetName, canary.Namespace, err)
	}

	// update service but keep the original destination weights
	if virtualService != nil {
		if diff := cmp.Diff(newSpec, virtualService.Spec, cmpopts.IgnoreFields(istiov1alpha3.DestinationWeight{}, "Weight")); diff != "" {
			vtClone := virtualService.DeepCopy()
			vtClone.Spec = newSpec

			_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Update(vtClone)
			if err != nil {
				return fmt.Errorf("VirtualService %s.%s update error %v", targetName, canary.Namespace, err)
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
	err error,
) {
	targetName := canary.Spec.TargetRef.Name
	vs := &istiov1alpha3.VirtualService{}
	vs, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			err = fmt.Errorf("VirtualService %s.%s not found", targetName, canary.Namespace)
			return
		}
		err = fmt.Errorf("VirtualService %s.%s query error %v", targetName, canary.Namespace, err)
		return
	}

	var httpRoute istiov1alpha3.HTTPRoute
	for _, http := range vs.Spec.Http {
		for _, r := range http.Route {
			if r.Destination.Host == fmt.Sprintf("%s-canary", targetName) {
				httpRoute = http
				break
			}
		}
	}

	for _, route := range httpRoute.Route {
		if route.Destination.Host == fmt.Sprintf("%s-primary", targetName) {
			primaryWeight = route.Weight
		}
		if route.Destination.Host == fmt.Sprintf("%s-canary", targetName) {
			canaryWeight = route.Weight
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("VirtualService %s.%s does not contain routes for %s-primary and %s-canary",
			targetName, canary.Namespace, targetName, targetName)
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (ir *IstioRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
) error {
	targetName := canary.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	vs, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("VirtualService %s.%s not found", targetName, canary.Namespace)

		}
		return fmt.Errorf("VirtualService %s.%s query error %v", targetName, canary.Namespace, err)
	}

	vsCopy := vs.DeepCopy()

	// weighted routing (progressive canary)
	vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
		{
			Match:         canary.Spec.Service.Match,
			Rewrite:       canary.Spec.Service.Rewrite,
			Timeout:       canary.Spec.Service.Timeout,
			Retries:       canary.Spec.Service.Retries,
			CorsPolicy:    canary.Spec.Service.CorsPolicy,
			AppendHeaders: addHeaders(canary),
			Route: []istiov1alpha3.DestinationWeight{
				makeDestination(canary, primaryName, primaryWeight),
				makeDestination(canary, canaryName, canaryWeight),
			},
		},
	}

	// fix routing (A/B testing)
	if len(canary.Spec.CanaryAnalysis.Match) > 0 {
		// merge the common routes with the canary ones
		canaryMatch := mergeMatchConditions(canary.Spec.CanaryAnalysis.Match, canary.Spec.Service.Match)
		vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
			{
				Match:         canaryMatch,
				Rewrite:       canary.Spec.Service.Rewrite,
				Timeout:       canary.Spec.Service.Timeout,
				Retries:       canary.Spec.Service.Retries,
				CorsPolicy:    canary.Spec.Service.CorsPolicy,
				AppendHeaders: addHeaders(canary),
				Route: []istiov1alpha3.DestinationWeight{
					makeDestination(canary, primaryName, primaryWeight),
					makeDestination(canary, canaryName, canaryWeight),
				},
			},
			{
				Match:         canary.Spec.Service.Match,
				Rewrite:       canary.Spec.Service.Rewrite,
				Timeout:       canary.Spec.Service.Timeout,
				Retries:       canary.Spec.Service.Retries,
				CorsPolicy:    canary.Spec.Service.CorsPolicy,
				AppendHeaders: addHeaders(canary),
				Route: []istiov1alpha3.DestinationWeight{
					makeDestination(canary, primaryName, primaryWeight),
				},
			},
		}
	}

	vs, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Update(vsCopy)
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s update failed: %v", targetName, canary.Namespace, err)

	}
	return nil
}

// addHeaders applies headers before forwarding a request to the destination service
// compatible with Istio 1.0.x and 1.1.0
func addHeaders(canary *flaggerv1.Canary) (headers map[string]string) {
	if canary.Spec.Service.Headers != nil &&
		canary.Spec.Service.Headers.Request != nil &&
		len(canary.Spec.Service.Headers.Request.Add) > 0 {
		headers = canary.Spec.Service.Headers.Request.Add
	}

	return
}

// mergeMatchConditions appends the URI match rules to canary conditions
func mergeMatchConditions(canary, defaults []istiov1alpha3.HTTPMatchRequest) []istiov1alpha3.HTTPMatchRequest {
	for i := range canary {
		for _, d := range defaults {
			if d.Uri != nil {
				canary[i].Uri = d.Uri
			}
		}
	}

	return canary
}

// makeDestination returns a an destination weight for the specified host
func makeDestination(canary *flaggerv1.Canary, host string, weight int) istiov1alpha3.DestinationWeight {
	dest := istiov1alpha3.DestinationWeight{
		Destination: istiov1alpha3.Destination{
			Host: host,
		},
		Weight: weight,
	}

	return dest
}
