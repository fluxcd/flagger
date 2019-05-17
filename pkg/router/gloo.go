package router

import (
	"context"
	"fmt"
	"strings"

	solokitclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	solokitcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	solokiterror "github.com/solo-io/solo-kit/pkg/errors"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// GlooRouter is managing Istio virtual services
type GlooRouter struct {
	ugClient            gloov1.UpstreamGroupClient
	logger              *zap.SugaredLogger
	upstreamDiscoveryNs string
}

func NewGlooRouter(ctx context.Context, provider string, flaggerClient clientset.Interface, logger *zap.SugaredLogger, cfg *rest.Config) (*GlooRouter, error) {
	// TODO if cfg is nil use memory client instead?
	sharedCache := kube.NewKubeCache(ctx)
	upstreamGroupClient, err := gloov1.NewUpstreamGroupClient(&factory.KubeResourceClientFactory{
		Crd:             gloov1.UpstreamGroupCrd,
		Cfg:             cfg,
		SharedCache:     sharedCache,
		SkipCrdCreation: true,
	})
	if err != nil {
		// this should never happen.
		return nil, fmt.Errorf("creating UpstreamGroup client %v", err)
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return nil, err
	}
	upstreamDiscoveryNs := ""
	if strings.HasPrefix(provider, "gloo:") {
		upstreamDiscoveryNs = strings.TrimPrefix(provider, "gloo:")
	}

	return NewGlooRouterWithClient(ctx, upstreamGroupClient, upstreamDiscoveryNs, logger), nil
}

func NewGlooRouterWithClient(ctx context.Context, routingRuleClient gloov1.UpstreamGroupClient, upstreamDiscoveryNs string, logger *zap.SugaredLogger) *GlooRouter {

	if upstreamDiscoveryNs == "" {
		upstreamDiscoveryNs = "gloo-system"
	}
	return &GlooRouter{ugClient: routingRuleClient, logger: logger, upstreamDiscoveryNs: upstreamDiscoveryNs}
}

// Reconcile creates or updates the Istio virtual service
func (gr *GlooRouter) Reconcile(canary *flaggerv1.Canary) error {
	// do we have routes already?
	if _, _, err := gr.GetRoutes(canary); err == nil {
		// we have routes, no need to do anything else
		return nil
	} else if solokiterror.IsNotExist(err) {
		return gr.SetRoutes(canary, 100, 0)
	} else {
		return err
	}
}

// GetRoutes returns the destinations weight for primary and canary
func (gr *GlooRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	err error,
) {
	targetName := canary.Spec.TargetRef.Name
	var ug *gloov1.UpstreamGroup
	ug, err = gr.ugClient.Read(canary.Namespace, targetName, solokitclients.ReadOpts{})
	if err != nil {
		return
	}

	dests := ug.GetDestinations()
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

// SetRoutes updates the destinations weight for primary and canary
func (gr *GlooRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
) error {
	targetName := canary.Spec.TargetRef.Name

	if primaryWeight == 0 && canaryWeight == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", targetName, canary.Namespace)
	}

	destinations := []*gloov1.WeightedDestination{}
	destinations = append(destinations, &gloov1.WeightedDestination{
		Destination: &gloov1.Destination{
			Upstream: solokitcore.ResourceRef{
				Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s-primary", targetName), canary.Spec.Service.Port),
				Namespace: gr.upstreamDiscoveryNs,
			},
		},
		Weight: uint32(primaryWeight),
	})

	destinations = append(destinations, &gloov1.WeightedDestination{
		Destination: &gloov1.Destination{
			Upstream: solokitcore.ResourceRef{
				Name:      upstreamName(canary.Namespace, fmt.Sprintf("%s-canary", targetName), canary.Spec.Service.Port),
				Namespace: gr.upstreamDiscoveryNs,
			},
		},
		Weight: uint32(canaryWeight),
	})

	upstreamGroup := &gloov1.UpstreamGroup{
		Metadata: solokitcore.Metadata{
			Name:      canary.Spec.TargetRef.Name,
			Namespace: canary.Namespace,
		},
		Destinations: destinations,
	}

	return gr.writeUpstreamGroupRuleForCanary(canary, upstreamGroup)
}

func (gr *GlooRouter) writeUpstreamGroupRuleForCanary(canary *flaggerv1.Canary, ug *gloov1.UpstreamGroup) error {
	targetName := canary.Spec.TargetRef.Name

	if oldUg, err := gr.ugClient.Read(ug.Metadata.Namespace, ug.Metadata.Name, solokitclients.ReadOpts{}); err != nil {
		if solokiterror.IsNotExist(err) {
			gr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("UpstreamGroup %s created", ug.Metadata.Name)
		} else {
			return fmt.Errorf("RoutingRule %s.%s read failed: %v", targetName, canary.Namespace, err)
		}
	} else {
		ug.Metadata.ResourceVersion = oldUg.Metadata.ResourceVersion
		// if the old and the new one are equal, no need to do anything.
		oldUg.Status = solokitcore.Status{}
		if oldUg.Equal(ug) {
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
	_, err := gr.ugClient.Write(ug, writeOpts)
	if err != nil {
		return fmt.Errorf("UpstreamGroup %s.%s update failed: %v", targetName, canary.Namespace, err)
	}
	return nil
}
