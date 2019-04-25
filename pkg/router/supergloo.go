package router

import (
	"context"
	"fmt"
	"strings"
	"time"

	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	solokitcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	solokiterror "github.com/solo-io/solo-kit/pkg/errors"

	types "github.com/gogo/protobuf/types"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	supergloov1alpha3 "github.com/solo-io/supergloo/pkg/api/external/istio/networking/v1alpha3"
	supergloov1 "github.com/solo-io/supergloo/pkg/api/v1"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// SuperglooRouter is managing Istio virtual services
type SuperglooRouter struct {
	rrClient   supergloov1.RoutingRuleClient
	logger     *zap.SugaredLogger
	targetMesh solokitcore.ResourceRef
}

func NewSuperglooRouter(ctx context.Context, provider string, flaggerClient clientset.Interface, logger *zap.SugaredLogger, cfg *rest.Config) (*SuperglooRouter, error) {
	// TODO if cfg is nil use memory client instead?
	sharedCache := kube.NewKubeCache(ctx)
	routingRuleClient, err := supergloov1.NewRoutingRuleClient(&factory.KubeResourceClientFactory{
		Crd:             supergloov1.RoutingRuleCrd,
		Cfg:             cfg,
		SharedCache:     sharedCache,
		SkipCrdCreation: true,
	})
	if err != nil {
		// this should never happen.
		return nil, fmt.Errorf("creating RoutingRule client %v", err)
	}
	if err := routingRuleClient.Register(); err != nil {
		return nil, err
	}

	// remove the supergloo: prefix
	provider = strings.TrimPrefix(provider, "supergloo:")
	// split name.namespace:
	parts := strings.Split(provider, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format for supergloo provider")
	}
	targetMesh := solokitcore.ResourceRef{
		Namespace: parts[1],
		Name:      parts[0],
	}
	return NewSuperglooRouterWithClient(ctx, routingRuleClient, targetMesh, logger), nil
}

func NewSuperglooRouterWithClient(ctx context.Context, routingRuleClient supergloov1.RoutingRuleClient, targetMesh solokitcore.ResourceRef, logger *zap.SugaredLogger) *SuperglooRouter {
	return &SuperglooRouter{rrClient: routingRuleClient, logger: logger, targetMesh: targetMesh}
}

// Reconcile creates or updates the Istio virtual service
func (sr *SuperglooRouter) Reconcile(canary *flaggerv1.Canary) error {

	if err := sr.setRetries(canary); err != nil {
		return err
	}
	if err := sr.setHeaders(canary); err != nil {
		return err
	}
	if err := sr.setCors(canary); err != nil {
		return err
	}

	// do we have routes already?
	if _, _, err := sr.GetRoutes(canary); err == nil {
		// we have routes, no need to do anything else
		return nil
	} else if solokiterror.IsNotExist(err) {
		return sr.SetRoutes(canary, 100, 0)
	} else {
		return err
	}
}

func (sr *SuperglooRouter) setRetries(canary *flaggerv1.Canary) error {
	if canary.Spec.Service.Retries == nil {
		return nil
	}
	retries, err := convertRetries(canary.Spec.Service.Retries)
	if err != nil {
		return err
	}
	rule := sr.createRule(canary, "retries", &supergloov1.RoutingRuleSpec{
		RuleType: &supergloov1.RoutingRuleSpec_Retries{
			Retries: retries,
		},
	})

	return sr.writeRuleForCanary(canary, rule)
}
func (sr *SuperglooRouter) setHeaders(canary *flaggerv1.Canary) error {
	if canary.Spec.Service.Headers == nil {
		return nil
	}
	headerManipulation, err := convertHeaders(canary.Spec.Service.Headers)
	if err != nil {
		return err
	}
	if headerManipulation == nil {
		return nil
	}
	rule := sr.createRule(canary, "headers", &supergloov1.RoutingRuleSpec{
		RuleType: &supergloov1.RoutingRuleSpec_HeaderManipulation{
			HeaderManipulation: headerManipulation,
		},
	})

	return sr.writeRuleForCanary(canary, rule)
}

func convertHeaders(headers *istiov1alpha3.Headers) (*supergloov1.HeaderManipulation, error) {
	var headersMaipulation *supergloov1.HeaderManipulation

	if headers.Request != nil {
		headersMaipulation = &supergloov1.HeaderManipulation{}

		headersMaipulation.RemoveRequestHeaders = headers.Request.Remove
		headersMaipulation.AppendRequestHeaders = make(map[string]string)
		for k, v := range headers.Request.Add {
			headersMaipulation.AppendRequestHeaders[k] = v
		}
	}
	if headers.Response != nil {
		if headersMaipulation == nil {
			headersMaipulation = &supergloov1.HeaderManipulation{}
		}

		headersMaipulation.RemoveResponseHeaders = headers.Response.Remove
		headersMaipulation.AppendResponseHeaders = make(map[string]string)
		for k, v := range headers.Response.Add {
			headersMaipulation.AppendResponseHeaders[k] = v
		}
	}

	return headersMaipulation, nil
}

func convertRetries(retries *istiov1alpha3.HTTPRetry) (*supergloov1.RetryPolicy, error) {
	perTryTimeout, err := time.ParseDuration(retries.PerTryTimeout)
	return &supergloov1.RetryPolicy{
		MaxRetries: &supergloov1alpha3.HTTPRetry{
			Attempts:      int32(retries.Attempts),
			PerTryTimeout: types.DurationProto(perTryTimeout),
			RetryOn:       retries.RetryOn,
		},
	}, err
}

func (sr *SuperglooRouter) setCors(canary *flaggerv1.Canary) error {
	corsPolicy := canary.Spec.Service.CorsPolicy
	if corsPolicy == nil {
		return nil
	}
	var maxAgeDuration *types.Duration
	if maxAge, err := time.ParseDuration(corsPolicy.MaxAge); err == nil {
		maxAgeDuration = types.DurationProto(maxAge)
	}

	rule := sr.createRule(canary, "cors", &supergloov1.RoutingRuleSpec{
		RuleType: &supergloov1.RoutingRuleSpec_CorsPolicy{
			CorsPolicy: &supergloov1alpha3.CorsPolicy{
				AllowOrigin:      corsPolicy.AllowOrigin,
				AllowMethods:     corsPolicy.AllowMethods,
				AllowHeaders:     corsPolicy.AllowHeaders,
				ExposeHeaders:    corsPolicy.ExposeHeaders,
				MaxAge:           maxAgeDuration,
				AllowCredentials: &types.BoolValue{Value: corsPolicy.AllowCredentials},
			},
		},
	})
	return sr.writeRuleForCanary(canary, rule)
}

func (sr *SuperglooRouter) createRule(canary *flaggerv1.Canary, namesuffix string, spec *supergloov1.RoutingRuleSpec) *supergloov1.RoutingRule {
	if namesuffix != "" {
		namesuffix = "-" + namesuffix
	}
	return &supergloov1.RoutingRule{
		Metadata: solokitcore.Metadata{
			Name:      canary.Spec.TargetRef.Name + namesuffix,
			Namespace: canary.Namespace,
		},
		TargetMesh: &sr.targetMesh,
		DestinationSelector: &supergloov1.PodSelector{
			SelectorType: &supergloov1.PodSelector_UpstreamSelector_{
				UpstreamSelector: &supergloov1.PodSelector_UpstreamSelector{
					Upstreams: []solokitcore.ResourceRef{{
						Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s", canary.Spec.TargetRef.Name), canary.Spec.Service.Port),
						Namespace: sr.targetMesh.Namespace,
					}},
				},
			},
		},
		Spec: spec,
	}
}

// GetRoutes returns the destinations weight for primary and canary
func (sr *SuperglooRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	err error,
) {
	targetName := canary.Spec.TargetRef.Name
	var rr *supergloov1.RoutingRule
	rr, err = sr.rrClient.Read(canary.Namespace, targetName, solokitclients.ReadOpts{})
	if err != nil {
		return
	}
	traffic := rr.GetSpec().GetTrafficShifting()
	if traffic == nil {
		err = fmt.Errorf("target rule is not for traffic shifting")
		return
	}
	dests := traffic.GetDestinations().GetDestinations()
	for _, dest := range dests {
		if dest.GetDestination().GetUpstream().Name == upstreamName(canary.Namespace, fmt.Sprintf("%s-primary", targetName), canary.Spec.Service.Port) {
			primaryWeight = int(dest.Weight)
		}
		if dest.GetDestination().GetUpstream().Name == upstreamName(canary.Namespace, fmt.Sprintf("%s-canary", targetName), canary.Spec.Service.Port) {
			canaryWeight = int(dest.Weight)
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("RoutingRule %s.%s does not contain routes for %s-primary and %s-canary",
			targetName, canary.Namespace, targetName, targetName)
	}

	return
}

func upstreamName(serviceNamespace, serviceName string, port int32) string {
	return fmt.Sprintf("%s-%s-%d", serviceNamespace, serviceName, port)
}

// SetRoutes updates the destinations weight for primary and canary
func (sr *SuperglooRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
) error {
	// upstream name is
	// in gloo-system
	// and is the same as
	targetName := canary.Spec.TargetRef.Name

	destinations := []*gloov1.WeightedDestination{}
	if primaryWeight != 0 {
		destinations = append(destinations, &gloov1.WeightedDestination{
			Destination: &gloov1.Destination{
				Upstream: solokitcore.ResourceRef{
					Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s-primary", targetName), canary.Spec.Service.Port),
					Namespace: sr.targetMesh.Namespace,
				},
			},
			Weight: uint32(primaryWeight),
		})
	}

	if canaryWeight != 0 {
		destinations = append(destinations, &gloov1.WeightedDestination{
			Destination: &gloov1.Destination{
				Upstream: solokitcore.ResourceRef{
					Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s-canary", targetName), canary.Spec.Service.Port),
					Namespace: sr.targetMesh.Namespace,
				},
			},
			Weight: uint32(canaryWeight),
		})
	}

	if len(destinations) == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", targetName, canary.Namespace)
	}

	rule := sr.createRule(canary, "", &supergloov1.RoutingRuleSpec{
		RuleType: &supergloov1.RoutingRuleSpec_TrafficShifting{
			TrafficShifting: &supergloov1.TrafficShifting{
				Destinations: &gloov1.MultiDestination{
					Destinations: destinations,
				},
			},
		},
	})

	return sr.writeRuleForCanary(canary, rule)
}

func (sr *SuperglooRouter) writeRuleForCanary(canary *flaggerv1.Canary, rule *supergloov1.RoutingRule) error {
	targetName := canary.Spec.TargetRef.Name

	if oldRr, err := sr.rrClient.Read(rule.Metadata.Namespace, rule.Metadata.Name, solokitclients.ReadOpts{}); err != nil {
		// ignore not exist errors..
		if !solokiterror.IsNotExist(err) {
			return fmt.Errorf("RoutingRule %s.%s read failed: %v", targetName, canary.Namespace, err)
		}
	} else {
		rule.Metadata.ResourceVersion = oldRr.Metadata.ResourceVersion
		// if the old and the new one are equal, no need to do anything.
		oldRr.Status = solokitcore.Status{}
		if oldRr.Equal(rule) {
			return nil
		}
	}

	kubeWriteOpts := &kube.KubeWriteOpts{
		PreWriteCallback: func(r *crdv1.Resource) {
			r.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
		},
	}
	writeOpts := solokitclients.WriteOpts{OverwriteExisting: true, StorageWriteOpts: kubeWriteOpts}
	_, err := sr.rrClient.Write(rule, writeOpts)
	if err != nil {
		return fmt.Errorf("RoutingRule %s.%s update failed: %v", targetName, canary.Namespace, err)
	}
	return nil
}
