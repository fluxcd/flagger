package router

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appmeshv1alpha1 "github.com/stefanprodan/flagger/pkg/apis/appmesh/v1alpha1"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// IstioRouter is managing Istio virtual services
type AppmeshRouter struct {
	kubeClient    kubernetes.Interface
	appMeshClient clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Sync creates or updates App Mesh virtual nodes and virtual services
func (ar *AppmeshRouter) Sync(canary *flaggerv1.Canary) error {
	if canary.Spec.Service.MeshName == "" {
		return fmt.Errorf("mesh name cannot be empty")
	}

	targetName := canary.Spec.TargetRef.Name
	targetHost := fmt.Sprintf("%s.%s", targetName, canary.Namespace)
	primaryName := fmt.Sprintf("%s-primary", targetName)
	primaryHost := fmt.Sprintf("%s.%s", primaryName, canary.Namespace)
	canaryName := fmt.Sprintf("%s-canary", targetName)

	// sync virtual node e.g. app-namespace
	// DNS app.namespace
	err := ar.syncVirtualNode(canary, fmt.Sprintf("%s-%s", targetName, canary.Namespace), primaryHost)
	if err != nil {
		return err
	}

	// sync virtual node e.g. app-primary-namespace
	// DNS app-primary.namespace
	err = ar.syncVirtualNode(canary, fmt.Sprintf("%s-%s", primaryName, canary.Namespace), primaryHost)
	if err != nil {
		return err
	}

	// sync virtual node e.g. app-canary-namespace
	// DNS app-canary.namespace
	err = ar.syncVirtualNode(canary, fmt.Sprintf("%s-%s", canaryName, canary.Namespace), targetHost)
	if err != nil {
		return err
	}

	// sync virtual service e.g. app.namespace
	// DNS app.namespace
	err = ar.syncVirtualService(canary, targetHost)
	if err != nil {
		return err
	}

	return nil
}

// syncVirtualNode creates or updates a virtual node
// the virtual node naming format is name-role-namespace
func (ar *AppmeshRouter) syncVirtualNode(canary *flaggerv1.Canary, name string, host string) error {
	backends := []appmeshv1alpha1.Backend{}
	for _, b := range canary.Spec.Service.Backends {
		backend := appmeshv1alpha1.Backend{
			VirtualService: appmeshv1alpha1.VirtualServiceBackend{
				VirtualServiceName: b,
			},
		}
		backends = append(backends, backend)
	}

	vnSpec := &appmeshv1alpha1.VirtualNodeSpec{
		MeshName: canary.Spec.Service.MeshName,
		Listeners: []appmeshv1alpha1.Listener{
			{
				PortMapping: appmeshv1alpha1.PortMapping{
					Port:     int64(canary.Spec.Service.Port),
					Protocol: "http",
				},
			},
		},
		ServiceDiscovery: &appmeshv1alpha1.ServiceDiscovery{
			Dns: &appmeshv1alpha1.DnsServiceDiscovery{
				HostName: host,
			},
		},
		Backends: backends,
	}

	virtualnode, err := ar.appMeshClient.AppmeshV1alpha1().VirtualNodes(canary.Namespace).Get(name, metav1.GetOptions{})

	// create virtual node
	if errors.IsNotFound(err) {
		virtualnode = &appmeshv1alpha1.VirtualNode{
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
			Spec: vnSpec,
		}
		_, err = ar.appMeshClient.AppmeshV1alpha1().VirtualNodes(canary.Namespace).Create(virtualnode)
		if err != nil {
			return fmt.Errorf("VirtualNode %s.%s create error %v", name, canary.Namespace, err)
		}
		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualNode %s.%s created", virtualnode.GetName(), canary.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("VirtualNode %s query error %v", name, err)
	}

	// update virtual node
	if virtualnode != nil {
		if diff := cmp.Diff(vnSpec, virtualnode.Spec); diff != "" {
			vnClone := virtualnode.DeepCopy()
			vnClone.Spec = vnSpec
			_, err = ar.appMeshClient.AppmeshV1alpha1().VirtualNodes(canary.Namespace).Update(vnClone)
			if err != nil {
				return fmt.Errorf("VirtualNode %s update error %v", name, err)
			}
			ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("VirtualNode %s updated", virtualnode.GetName())
		}
	}

	return nil
}

func (ar *AppmeshRouter) syncVirtualService(canary *flaggerv1.Canary, name string) error {
	targetName := canary.Spec.TargetRef.Name
	canaryVirtualNode := fmt.Sprintf("%s-canary-%s", targetName, canary.Namespace)
	primaryVirtualNode := fmt.Sprintf("%s-primary-%s", targetName, canary.Namespace)

	// App Mesh supports only URI prefix
	routePrefix := "/"
	if len(canary.Spec.Service.Match) > 0 &&
		canary.Spec.Service.Match[0].Uri != nil &&
		canary.Spec.Service.Match[0].Uri.Prefix != "" {
		routePrefix = canary.Spec.Service.Match[0].Uri.Prefix
	}

	vsSpec := &appmeshv1alpha1.VirtualServiceSpec{
		MeshName: canary.Spec.Service.MeshName,
		VirtualRouter: &appmeshv1alpha1.VirtualRouter{
			Name: fmt.Sprintf("%s-router", name),
		},
		Routes: []appmeshv1alpha1.Route{
			{
				Name: fmt.Sprintf("%s-route", name),
				Http: appmeshv1alpha1.HttpRoute{
					Match: appmeshv1alpha1.HttpRouteMatch{
						Prefix: routePrefix,
					},
					Action: appmeshv1alpha1.HttpRouteAction{
						WeightedTargets: []appmeshv1alpha1.WeightedTarget{
							{
								VirtualNodeName: canaryVirtualNode,
								Weight:          0,
							},
							{
								VirtualNodeName: primaryVirtualNode,
								Weight:          100,
							},
						},
					},
				},
			},
		},
	}

	virtualService, err := ar.appMeshClient.AppmeshV1alpha1().VirtualServices(canary.Namespace).Get(name, metav1.GetOptions{})

	// create virtual service
	if errors.IsNotFound(err) {
		virtualService = &appmeshv1alpha1.VirtualService{
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
			Spec: vsSpec,
		}
		_, err = ar.appMeshClient.AppmeshV1alpha1().VirtualServices(canary.Namespace).Create(virtualService)
		if err != nil {
			return fmt.Errorf("VirtualService %s create error %v", name, err)
		}
		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualService %s created", virtualService.GetName())
		return nil
	}

	if err != nil {
		return fmt.Errorf("VirtualService %s query error %v", name, err)
	}

	// update virtual service but keep the original target weights
	if virtualService != nil {
		if diff := cmp.Diff(vsSpec, virtualService.Spec, cmpopts.IgnoreTypes(appmeshv1alpha1.WeightedTarget{})); diff != "" {
			vsClone := virtualService.DeepCopy()
			vsClone.Spec = vsSpec
			vsClone.Spec.Routes[0].Http.Action = virtualService.Spec.Routes[0].Http.Action

			_, err = ar.appMeshClient.AppmeshV1alpha1().VirtualServices(canary.Namespace).Update(vsClone)
			if err != nil {
				return fmt.Errorf("VirtualService %s update error %v", name, err)
			}
			ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("VirtualService %s updated", virtualService.GetName())
		}
	}

	return nil
}
