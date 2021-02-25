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
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	appmeshv1 "github.com/fluxcd/flagger/pkg/apis/appmesh/v1beta2"
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// AppMeshRouter is managing AppMesh virtual services
type AppMeshv1beta2Router struct {
	kubeClient    kubernetes.Interface
	appmeshClient clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	labelSelector string
}

// Reconcile creates or updates App Mesh virtual nodes and virtual services
func (ar *AppMeshv1beta2Router) Reconcile(canary *flaggerv1.Canary) error {
	svcSuffix := "svc.cluster.local."
	apexName, primaryName, canaryName := canary.GetServiceNames()
	primaryHost := fmt.Sprintf("%s.%s.%s", primaryName, canary.Namespace, svcSuffix)
	canaryHost := fmt.Sprintf("%s.%s.%s", canaryName, canary.Namespace, svcSuffix)

	// sync virtual node e.g. app-namespace
	// DNS app.namespace
	//err := ar.reconcileVirtualNode(canary, apexName, fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name), primaryHost)
	//if err != nil {
	//	return fmt.Errorf("reconcileVirtualNode failed: %w", err)
	//}

	// sync virtual node e.g. app-primary-namespace
	// DNS app-primary.namespace
	err := ar.reconcileVirtualNode(canary, primaryName, fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name), primaryHost)
	if err != nil {
		return fmt.Errorf("reconcileVirtualNode failed: %w", err)
	}

	// sync virtual node e.g. app-canary-namespace
	// DNS app-canary.namespace
	err = ar.reconcileVirtualNode(canary, canaryName, canary.Spec.TargetRef.Name, canaryHost)
	if err != nil {
		return fmt.Errorf("reconcileVirtualNode failed: %w", err)
	}

	// sync main virtual router
	// DNS app.namespace
	err = ar.reconcileVirtualRouter(canary, apexName, 0)
	if err != nil {
		return fmt.Errorf("reconcileVirtualService failed: %w", err)
	}

	// sync canary virtual router
	// DNS app-canary.namespace
	err = ar.reconcileVirtualRouter(canary, canaryName, 100)
	if err != nil {
		return fmt.Errorf("reconcileVirtualRouter failed: %w", err)
	}

	return nil
}

// reconcileVirtualNode creates or updates a virtual node
// the virtual node naming format is name-role-namespace
func (ar *AppMeshv1beta2Router) reconcileVirtualNode(canary *flaggerv1.Canary, name string, podSelector string, host string) error {
	protocol := ar.getProtocol(canary)
	timeout := ar.makeListenerTimeout(canary)

	vnSpec := appmeshv1.VirtualNodeSpec{
		Listeners: []appmeshv1.Listener{
			{
				PortMapping: appmeshv1.PortMapping{
					Port:     ar.getContainerPort(canary),
					Protocol: protocol,
				},
				Timeout: timeout,
			},
		},
		ServiceDiscovery: &appmeshv1.ServiceDiscovery{
			DNS: &appmeshv1.DNSServiceDiscovery{
				Hostname: host,
			},
		},
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				ar.labelSelector: podSelector,
			},
		},
	}

	backends := make([]appmeshv1.Backend, 0)
	for i := range canary.Spec.Service.Backends {
		if strings.HasPrefix(canary.Spec.Service.Backends[i], "arn:aws") {
			backends = append(backends, appmeshv1.Backend{
				VirtualService: appmeshv1.VirtualServiceBackend{
					VirtualServiceARN: &canary.Spec.Service.Backends[i],
				},
			})
		} else {
			backends = append(backends, appmeshv1.Backend{
				VirtualService: appmeshv1.VirtualServiceBackend{
					VirtualServiceRef: &appmeshv1.VirtualServiceReference{
						Name: canary.Spec.Service.Backends[i],
					},
				},
			})
		}
	}

	if len(backends) > 0 {
		vnSpec.Backends = backends
	}

	virtualnode, err := ar.appmeshClient.AppmeshV1beta2().VirtualNodes(canary.Namespace).Get(context.TODO(), name, metav1.GetOptions{})

	// create virtual node
	if errors.IsNotFound(err) {
		virtualnode = &appmeshv1.VirtualNode{
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
		_, err = ar.appmeshClient.AppmeshV1beta2().VirtualNodes(canary.Namespace).Create(context.TODO(), virtualnode, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("VirtualNode %s.%s create error %w", name, canary.Namespace, err)
		}
		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualNode %s.%s created", virtualnode.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("VirtualNode %s get query error %w", name, err)
	}

	// update virtual node
	if virtualnode != nil {
		if diff := cmp.Diff(vnSpec, virtualnode.Spec,
			cmpopts.IgnoreFields(appmeshv1.VirtualNodeSpec{}, "AWSName", "MeshRef")); diff != "" {
			vnClone := virtualnode.DeepCopy()
			vnClone.Spec = vnSpec
			vnClone.Spec.AWSName = virtualnode.Spec.AWSName
			vnClone.Spec.MeshRef = virtualnode.Spec.MeshRef
			_, err = ar.appmeshClient.AppmeshV1beta2().VirtualNodes(canary.Namespace).Update(context.TODO(), vnClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("VirtualNode %s update error %w", name, err)
			}
			ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("VirtualNode %s updated", virtualnode.GetName())
		}
	}

	return nil
}

// reconcileVirtualRouter creates or updates a virtual router
func (ar *AppMeshv1beta2Router) reconcileVirtualRouter(canary *flaggerv1.Canary, name string, canaryWeight int64) error {
	apexName, _, _ := canary.GetServiceNames()
	canaryVirtualNode := fmt.Sprintf("%s-canary", apexName)
	primaryVirtualNode := fmt.Sprintf("%s-primary", apexName)
	protocol := ar.getProtocol(canary)
	timeout := ar.makeRouteTimeout(canary)

	routerName := apexName
	if canaryWeight > 0 {
		routerName = fmt.Sprintf("%s-canary", apexName)
	}
	// App Mesh supports only URI prefix
	routePrefix := "/"
	if len(canary.Spec.Service.Match) > 0 &&
		canary.Spec.Service.Match[0].Uri != nil &&
		canary.Spec.Service.Match[0].Uri.Prefix != "" {
		routePrefix = canary.Spec.Service.Match[0].Uri.Prefix
	}

	// Canary progressive traffic shift
	routes := []appmeshv1.Route{
		{
			Name: routerName,
			HTTPRoute: &appmeshv1.HTTPRoute{
				Match: appmeshv1.HTTPRouteMatch{
					Prefix: routePrefix,
				},
				Timeout:     timeout,
				RetryPolicy: ar.makeRetryPolicy(canary),
				Action: appmeshv1.HTTPRouteAction{
					WeightedTargets: []appmeshv1.WeightedTarget{
						{
							VirtualNodeRef: &appmeshv1.VirtualNodeReference{
								Name: canaryVirtualNode,
							},
							Weight: canaryWeight,
						},
						{
							VirtualNodeRef: &appmeshv1.VirtualNodeReference{
								Name: primaryVirtualNode,
							},
							Weight: 100 - canaryWeight,
						},
					},
				},
			},
		},
	}

	// A/B testing - header based routing
	if len(canary.GetAnalysis().Match) > 0 && canaryWeight == 0 {
		routes = []appmeshv1.Route{
			{
				Name:     fmt.Sprintf("%s-a", apexName),
				Priority: int64p(10),
				HTTPRoute: &appmeshv1.HTTPRoute{
					Match: appmeshv1.HTTPRouteMatch{
						Prefix:  routePrefix,
						Headers: ar.makeHeaders(canary),
					},
					Timeout:     timeout,
					RetryPolicy: ar.makeRetryPolicy(canary),
					Action: appmeshv1.HTTPRouteAction{
						WeightedTargets: []appmeshv1.WeightedTarget{
							{
								VirtualNodeRef: &appmeshv1.VirtualNodeReference{
									Name: canaryVirtualNode,
								},
								Weight: canaryWeight,
							},
							{
								VirtualNodeRef: &appmeshv1.VirtualNodeReference{
									Name: primaryVirtualNode,
								},
								Weight: 100 - canaryWeight,
							},
						},
					},
				},
			},
			{
				Name:     fmt.Sprintf("%s-b", apexName),
				Priority: int64p(20),
				HTTPRoute: &appmeshv1.HTTPRoute{
					Match: appmeshv1.HTTPRouteMatch{
						Prefix: routePrefix,
					},
					Timeout:     timeout,
					RetryPolicy: ar.makeRetryPolicy(canary),
					Action: appmeshv1.HTTPRouteAction{
						WeightedTargets: []appmeshv1.WeightedTarget{
							{
								VirtualNodeRef: &appmeshv1.VirtualNodeReference{
									Name: primaryVirtualNode,
								},
								Weight: 100,
							},
						},
					},
				},
			},
		}
	}

	vrSpec := appmeshv1.VirtualRouterSpec{
		Listeners: []appmeshv1.VirtualRouterListener{
			{
				PortMapping: appmeshv1.PortMapping{
					Port:     ar.getContainerPort(canary),
					Protocol: protocol,
				},
			},
		},
		Routes: routes,
	}

	virtualRouter, err := ar.appmeshClient.AppmeshV1beta2().VirtualRouters(canary.Namespace).Get(context.TODO(), name, metav1.GetOptions{})

	// create virtual router
	if errors.IsNotFound(err) {
		virtualRouter = &appmeshv1.VirtualRouter{
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
			Spec: vrSpec,
		}

		_, err = ar.appmeshClient.AppmeshV1beta2().VirtualRouters(canary.Namespace).Create(context.TODO(), virtualRouter, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("VirtualRouter %s create error %w", name, err)
		}
		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualRouter %s created", virtualRouter.GetName())

		virtualService := &appmeshv1.VirtualService{
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
			Spec: appmeshv1.VirtualServiceSpec{
				Provider: &appmeshv1.VirtualServiceProvider{
					VirtualRouter: &appmeshv1.VirtualRouterServiceProvider{
						VirtualRouterRef: &appmeshv1.VirtualRouterReference{
							Name: name,
						},
					},
				},
			},
		}

		// set App Mesh Gateway annotation on primary virtual service
		if canaryWeight == 0 {
			a := ar.gatewayAnnotations(canary)
			if len(a) > 0 {
				virtualService.ObjectMeta.Annotations = a
			}
		}

		_, err = ar.appmeshClient.AppmeshV1beta2().VirtualServices(canary.Namespace).Create(context.TODO(), virtualService, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("VirtualService %s create error %w", name, err)
		}
		ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("VirtualService %s created", virtualRouter.GetName())

		return nil
	} else if err != nil {
		return fmt.Errorf("VirtualRouter %s get query error: %w", name, err)
	}

	// update virtual router but keep the original target weights
	if virtualRouter != nil {
		if diff := cmp.Diff(vrSpec, virtualRouter.Spec,
			cmpopts.IgnoreFields(appmeshv1.VirtualRouterSpec{}, "AWSName", "MeshRef"),
			cmpopts.IgnoreTypes(appmeshv1.WeightedTarget{}, appmeshv1.MeshReference{})); diff != "" {
			vrClone := virtualRouter.DeepCopy()
			vrClone.Spec = vrSpec
			vrClone.Spec.Routes[0].HTTPRoute.Action = virtualRouter.Spec.Routes[0].HTTPRoute.Action
			vrClone.Spec.AWSName = virtualRouter.Spec.AWSName
			vrClone.Spec.MeshRef = virtualRouter.Spec.MeshRef
			_, err = ar.appmeshClient.AppmeshV1beta2().VirtualRouters(canary.Namespace).Update(context.TODO(), vrClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("VirtualRouter %s update error: %w", name, err)
			}
			ar.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("VirtualRouter %s updated", virtualRouter.GetName())
		}
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (ar *AppMeshv1beta2Router) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	virtualRouter, err := ar.appmeshClient.AppmeshV1beta2().VirtualRouters(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("VirtualRouter %s get query error: %w", apexName, err)
		return
	}

	if len(virtualRouter.Spec.Routes) < 1 || len(virtualRouter.Spec.Routes[0].HTTPRoute.Action.WeightedTargets) != 2 {
		err = fmt.Errorf("VirtualRouter routes %s not found", apexName)
		return
	}

	targets := virtualRouter.Spec.Routes[0].HTTPRoute.Action.WeightedTargets
	for _, t := range targets {
		if t.VirtualNodeRef.Name == canaryName {
			canaryWeight = int(t.Weight)
		}
		if t.VirtualNodeRef.Name == primaryName {
			primaryWeight = int(t.Weight)
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("VirtualRouter %s does not contain routes for %s-primary and %s-canary",
			apexName, apexName, apexName)
	}

	mirrored = false

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (ar *AppMeshv1beta2Router) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	virtualRouter, err := ar.appmeshClient.AppmeshV1beta2().VirtualRouters(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("VirtualRouter %s get query error: %w", apexName, err)
	}

	vrClone := virtualRouter.DeepCopy()
	vrClone.Spec.Routes[0].HTTPRoute.Action = appmeshv1.HTTPRouteAction{
		WeightedTargets: []appmeshv1.WeightedTarget{
			{
				VirtualNodeRef: &appmeshv1.VirtualNodeReference{
					Name: canaryName,
				},
				Weight: int64(canaryWeight),
			},
			{
				VirtualNodeRef: &appmeshv1.VirtualNodeReference{
					Name: primaryName,
				},
				Weight: int64(primaryWeight),
			},
		},
	}

	_, err = ar.appmeshClient.AppmeshV1beta2().VirtualRouters(canary.Namespace).Update(context.TODO(), vrClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("VirtualRouter %s update error: %w", apexName, err)
	}
	return nil
}

// getTimeout converts the Canary.Service.Timeout to AppMesh Duration
func (ar *AppMeshv1beta2Router) getTimeout(canary *flaggerv1.Canary) *appmeshv1.Duration {
	if canary.Spec.Service.Timeout != "" {
		if d, err := time.ParseDuration(canary.Spec.Service.Timeout); err == nil {
			return &appmeshv1.Duration{
				Unit:  appmeshv1.DurationUnitMS,
				Value: d.Milliseconds(),
			}
		}
	}
	return nil
}

// makeRouteTimeout creates an AppMesh HTTPTimeout from the Canary.Service.Timeout
func (ar *AppMeshv1beta2Router) makeRouteTimeout(canary *flaggerv1.Canary) *appmeshv1.HTTPTimeout {
	if timeout := ar.getTimeout(canary); timeout != nil {
		return &appmeshv1.HTTPTimeout{
			PerRequest: timeout,
		}
	}
	return nil
}

// makeListenerTimeout creates an AppMesh ListenerTimeout from the Canary.Service.Timeout
func (ar *AppMeshv1beta2Router) makeListenerTimeout(canary *flaggerv1.Canary) *appmeshv1.ListenerTimeout {
	if timeout := ar.makeRouteTimeout(canary); timeout != nil {
		return &appmeshv1.ListenerTimeout{
			HTTP: timeout,
		}
	}
	return nil
}

// makeRetryPolicy creates an AppMesh HTTPRetryPolicy from the Canary.Service.Retries
// default: one retry on gateway error with a 250ms timeout
func (ar *AppMeshv1beta2Router) makeRetryPolicy(canary *flaggerv1.Canary) *appmeshv1.HTTPRetryPolicy {
	if canary.Spec.Service.Retries != nil {
		timeout := int64(250)
		if d, err := time.ParseDuration(canary.Spec.Service.Retries.PerTryTimeout); err == nil {
			timeout = d.Milliseconds()
		}

		attempts := 1
		if canary.Spec.Service.Retries.Attempts > 0 {
			attempts = canary.Spec.Service.Retries.Attempts
		}
		retryPolicy := &appmeshv1.HTTPRetryPolicy{
			PerRetryTimeout: appmeshv1.Duration{
				Unit:  appmeshv1.DurationUnitMS,
				Value: timeout,
			},
			MaxRetries: int64(attempts),
		}

		events := []string{"gateway-error"}
		if len(canary.Spec.Service.Retries.RetryOn) > 0 {
			events = strings.Split(canary.Spec.Service.Retries.RetryOn, ",")
		}
		for _, value := range events {
			retryPolicy.HTTPRetryEvents = append(retryPolicy.HTTPRetryEvents, appmeshv1.HTTPRetryPolicyEvent(value))
		}
		return retryPolicy
	}

	return nil
}

// makeRetryPolicy creates an App Mesh HttpRouteHeader from the Canary.CanaryAnalysis.Match
func (ar *AppMeshv1beta2Router) makeHeaders(canary *flaggerv1.Canary) []appmeshv1.HTTPRouteHeader {

	var headers []appmeshv1.HTTPRouteHeader
	for _, m := range canary.GetAnalysis().Match {
		for key, value := range m.Headers {
			header := appmeshv1.HTTPRouteHeader{
				Name: key,
				Match: &appmeshv1.HeaderMatchMethod{
					Exact:  stringp(value.Exact),
					Prefix: stringp(value.Prefix),
					Regex:  stringp(value.Regex),
					Suffix: stringp(value.Suffix),
				},
			}
			headers = append(headers, header)
		}
	}

	return headers
}

func (ar *AppMeshv1beta2Router) getProtocol(canary *flaggerv1.Canary) appmeshv1.PortProtocol {
	if strings.Contains(canary.Spec.Service.PortName, "grpc") {
		return appmeshv1.PortProtocolGRPC
	}
	return appmeshv1.PortProtocolHTTP
}

func (ar *AppMeshv1beta2Router) getContainerPort(canary *flaggerv1.Canary) appmeshv1.PortNumber {
	containerPort := canary.Spec.Service.Port
	if canary.Spec.Service.TargetPort.IntVal > 0 {
		containerPort = canary.Spec.Service.TargetPort.IntVal
	}
	return appmeshv1.PortNumber(containerPort)
}

func (ar *AppMeshv1beta2Router) gatewayAnnotations(canary *flaggerv1.Canary) map[string]string {
	a := make(map[string]string)
	domains := ""
	for _, value := range canary.Spec.Service.Hosts {
		domains += value + ","
	}
	if domains != "" {
		a["gateway.appmesh.k8s.aws/expose"] = "true"
		a["gateway.appmesh.k8s.aws/domain"] = domains
		if canary.Spec.Service.Timeout != "" {
			a["gateway.appmesh.k8s.aws/timeout"] = canary.Spec.Service.Timeout
		}
		if canary.Spec.Service.Retries != nil && canary.Spec.Service.Retries.Attempts > 0 {
			a["gateway.appmesh.k8s.aws/retries"] = strconv.Itoa(canary.Spec.Service.Retries.Attempts)
		}
	}
	return a
}

func (ar *AppMeshv1beta2Router) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
