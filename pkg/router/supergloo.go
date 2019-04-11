package router

import (
	"context"
	"fmt"

	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	solokitcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	supergloov1 "github.com/solo-io/supergloo/pkg/api/v1"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

// SuperglooRouter is managing Istio virtual services
type SuperglooRouter struct {
	kubeClient kubernetes.Interface

	rrClient      supergloov1.RoutingRuleClient
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func (ir *SuperglooRouter) foo() error {

	sharedCache := kube.NewKubeCache(context.TODO())
	RoutingRuleClient, err := supergloov1.NewRoutingRuleClient(&factory.KubeResourceClientFactory{
		Crd: supergloov1.RoutingRuleCrd,
		//		Cfg:         ir.kubeClient,
		SharedCache: sharedCache,
	})
	if err != nil {
		return fmt.Errorf("creating RoutingRule client %v", err)
	}
	if err := RoutingRuleClient.Register(); err != nil {
		return err
	}
	ir.rrClient = RoutingRuleClient
	return nil
}

// Reconcile creates or updates the Istio virtual service
func (ir *SuperglooRouter) Reconcile(canary *flaggerv1.Canary) error {
	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (ir *SuperglooRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	err error,
) {
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

	rule := &supergloov1.RoutingRule{
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
						Destinations: []*gloov1.WeightedDestination{
							{
								Destination: &gloov1.Destination{
									Upstream: solokitcore.ResourceRef{
										Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s-primary", targetName), canary.Spec.Service.Port),
										Namespace: "supergloo-system",
									},
								},
								Weight: uint32(primaryWeight),
							}, {
								Destination: &gloov1.Destination{
									Upstream: solokitcore.ResourceRef{
										Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s-canary", targetName), canary.Spec.Service.Port),
										Namespace: "supergloo-system",
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

	// TODO: read to get the resource version
	_, err := ir.rrClient.Write(rule, solokitclients.WriteOpts{OverwriteExisting: true})
	if err != nil {
		return fmt.Errorf("RoutingRule %s.%s update failed: %v", targetName, canary.Namespace, err)

	}
	return nil
}
