package router

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	istiov1alpha3 "github.com/stefanprodan/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Sync creates or updates the Istio virtual service
func (ir *IstioRouter) Sync(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	// set hosts and add the ClusterIP service host if it doesn't exists
	hosts := cd.Spec.Service.Hosts
	var hasServiceHost bool
	for _, h := range hosts {
		if h == targetName {
			hasServiceHost = true
			break
		}
	}
	if !hasServiceHost {
		hosts = append(hosts, targetName)
	}

	// set gateways and add the mesh gateway if it doesn't exists
	gateways := cd.Spec.Service.Gateways
	var hasMeshGateway bool
	for _, g := range gateways {
		if g == "mesh" {
			hasMeshGateway = true
			break
		}
	}
	if !hasMeshGateway {
		gateways = append(gateways, "mesh")
	}

	// create destinations with primary weight 100% and canary weight 0%
	route := []istiov1alpha3.DestinationWeight{
		{
			Destination: istiov1alpha3.Destination{
				Host: primaryName,
				Port: istiov1alpha3.PortSelector{
					Number: uint32(cd.Spec.Service.Port),
				},
			},
			Weight: 100,
		},
		{
			Destination: istiov1alpha3.Destination{
				Host: fmt.Sprintf("%s-canary", targetName),
				Port: istiov1alpha3.PortSelector{
					Number: uint32(cd.Spec.Service.Port),
				},
			},
			Weight: 0,
		},
	}
	newSpec := istiov1alpha3.VirtualServiceSpec{
		Hosts:    hosts,
		Gateways: gateways,
		Http: []istiov1alpha3.HTTPRoute{
			{
				Match:         cd.Spec.Service.Match,
				Rewrite:       cd.Spec.Service.Rewrite,
				Timeout:       cd.Spec.Service.Timeout,
				Retries:       cd.Spec.Service.Retries,
				AppendHeaders: cd.Spec.Service.AppendHeaders,
				Route:         route,
			},
		},
	}

	virtualService, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Get(targetName, metav1.GetOptions{})
	// insert
	if errors.IsNotFound(err) {
		virtualService = &istiov1alpha3.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: newSpec,
		}
		_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Create(virtualService)
		if err != nil {
			return fmt.Errorf("VirtualService %s.%s create error %v", targetName, cd.Namespace, err)
		}
		ir.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
			Infof("VirtualService %s.%s created", virtualService.GetName(), cd.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("VirtualService %s.%s query error %v", targetName, cd.Namespace, err)
	}

	// update service but keep the original destination weights
	if virtualService != nil {
		if diff := cmp.Diff(newSpec, virtualService.Spec, cmpopts.IgnoreTypes(istiov1alpha3.DestinationWeight{})); diff != "" {
			vtClone := virtualService.DeepCopy()
			vtClone.Spec = newSpec
			if len(virtualService.Spec.Http) > 0 {
				vtClone.Spec.Http[0].Route = virtualService.Spec.Http[0].Route
			}
			_, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(cd.Namespace).Update(vtClone)
			if err != nil {
				return fmt.Errorf("VirtualService %s.%s update error %v", targetName, cd.Namespace, err)
			}
			ir.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("VirtualService %s.%s updated", virtualService.GetName(), cd.Namespace)
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
	vs, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(targetName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			err = fmt.Errorf("VirtualService %s.%s not found", targetName, canary.Namespace)
			return
		}
		err = fmt.Errorf("VirtualService %s.%s query error %v", targetName, canary.Namespace, err)
		return
	}

	for _, http := range vs.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == fmt.Sprintf("%s-primary", targetName) {
				primaryWeight = route.Weight
			}
			if route.Destination.Host == fmt.Sprintf("%s-canary", targetName) {
				canaryWeight = route.Weight
			}
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
	vs, err := ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Get(targetName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("VirtualService %s.%s not found", targetName, canary.Namespace)

		}
		return fmt.Errorf("VirtualService %s.%s query error %v", targetName, canary.Namespace, err)
	}

	vsCopy := vs.DeepCopy()
	vsCopy.Spec.Http = []istiov1alpha3.HTTPRoute{
		{
			Match:         canary.Spec.Service.Match,
			Rewrite:       canary.Spec.Service.Rewrite,
			Timeout:       canary.Spec.Service.Timeout,
			Retries:       canary.Spec.Service.Retries,
			AppendHeaders: canary.Spec.Service.AppendHeaders,
			Route: []istiov1alpha3.DestinationWeight{
				{
					Destination: istiov1alpha3.Destination{
						Host: fmt.Sprintf("%s-primary", targetName),
						Port: istiov1alpha3.PortSelector{
							Number: uint32(canary.Spec.Service.Port),
						},
					},
					Weight: primaryWeight,
				},
				{
					Destination: istiov1alpha3.Destination{
						Host: fmt.Sprintf("%s-canary", targetName),
						Port: istiov1alpha3.PortSelector{
							Number: uint32(canary.Spec.Service.Port),
						},
					},
					Weight: canaryWeight,
				},
			},
		},
	}

	vs, err = ir.istioClient.NetworkingV1alpha3().VirtualServices(canary.Namespace).Update(vsCopy)
	if err != nil {
		return fmt.Errorf("VirtualService %s.%s update failed: %v", targetName, canary.Namespace, err)

	}
	return nil
}
