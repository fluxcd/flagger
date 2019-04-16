package router

import (
	"context"
	"fmt"

	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	solokitcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	solokiterror "github.com/solo-io/solo-kit/pkg/errors"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	supergloov1 "github.com/solo-io/supergloo/pkg/api/v1"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

// SuperglooRouter is managing Istio virtual services
type SuperglooRouter struct {
	rrClient   supergloov1.RoutingRuleClient
	logger     *zap.SugaredLogger
	targetMesh solokitcore.ResourceRef
}

func NewSuperglooRouter(ctx context.Context, flaggerClient clientset.Interface, logger *zap.SugaredLogger, cfg *rest.Config) (*SuperglooRouter, error) {

	sharedCache := kube.NewKubeCache(ctx)
	routingRuleClient, err := supergloov1.NewRoutingRuleClient(&factory.KubeResourceClientFactory{
		Crd:             supergloov1.RoutingRuleCrd,
		Cfg:             cfg,
		SharedCache:     sharedCache,
		SkipCrdCreation: true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating RoutingRule client %v", err)
	}
	if err := routingRuleClient.Register(); err != nil {
		return nil, err
	}
	// TODO(yuval-k): un hard code this
	targetMesh := solokitcore.ResourceRef{
		Namespace: "supergloo-system",
		Name:      "yuval",
	}
	return NewSuperglooRouterWithClient(ctx, routingRuleClient, targetMesh, logger), nil
}

func NewSuperglooRouterWithClient(ctx context.Context, routingRuleClient supergloov1.RoutingRuleClient, targetMesh solokitcore.ResourceRef, logger *zap.SugaredLogger) *SuperglooRouter {
	return &SuperglooRouter{rrClient: routingRuleClient, logger: logger, targetMesh: targetMesh}
}

// Reconcile creates or updates the Istio virtual service
func (ir *SuperglooRouter) Reconcile(canary *flaggerv1.Canary) error {
	// TODO: add header rules
	// TODO: add retry rules
	// TODO: add CORS rules
	// TODO: maybe more? rewrite \ timeout?
	return ir.SetRoutes(canary, 100, 0)
}

// GetRoutes returns the destinations weight for primary and canary
func (ir *SuperglooRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	err error,
) {
	targetName := canary.Spec.TargetRef.Name
	var rr *supergloov1.RoutingRule
	rr, err = ir.rrClient.Read(canary.Namespace, targetName, solokitclients.ReadOpts{})
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
func (ir *SuperglooRouter) SetRoutes(
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
					Namespace: "supergloo-system",
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
					Namespace: "supergloo-system",
				},
			},
			Weight: uint32(canaryWeight),
		})
	}

	if len(destinations) == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", targetName, canary.Namespace)
	}

	rule := &supergloov1.RoutingRule{
		Metadata: solokitcore.Metadata{
			Name:      targetName,
			Namespace: canary.Namespace,
		},
		TargetMesh: &ir.targetMesh,
		DestinationSelector: &supergloov1.PodSelector{
			SelectorType: &supergloov1.PodSelector_UpstreamSelector_{
				UpstreamSelector: &supergloov1.PodSelector_UpstreamSelector{
					Upstreams: []solokitcore.ResourceRef{{
						Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s", targetName), canary.Spec.Service.Port),
						Namespace: "supergloo-system",
					}},
				},
			},
		},
		Spec: &supergloov1.RoutingRuleSpec{
			RuleType: &supergloov1.RoutingRuleSpec_TrafficShifting{
				TrafficShifting: &supergloov1.TrafficShifting{
					Destinations: &gloov1.MultiDestination{
						Destinations: destinations,
					},
				},
			},
		},
	}

	if oldRr, err := ir.rrClient.Read(rule.Metadata.Namespace, rule.Metadata.Name, solokitclients.ReadOpts{}); err != nil {
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

	_, err := ir.rrClient.Write(rule, solokitclients.WriteOpts{OverwriteExisting: true})
	if err != nil {
		return fmt.Errorf("RoutingRule %s.%s update failed: %v", targetName, canary.Namespace, err)
	}
	return nil
}
