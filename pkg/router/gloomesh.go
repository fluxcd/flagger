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

	v2 "github.com/fluxcd/flagger/pkg/apis/gloo/networking/v2"
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

const (
	WeightedRouteName = "weighted-route"
)

// GlooMeshRouter is managing Gloo Mesh route tables
type GlooMeshRouter struct {
	kubeClient         kubernetes.Interface
	flaggerClient      clientset.Interface
	logger             *zap.SugaredLogger
	includeLabelPrefix []string
}

// Reconcile creates or updates the delegate RouteTable
func (gmr *GlooMeshRouter) Reconcile(canary *flaggerv1.Canary) error {
	if err := gmr.reconcileRouteTable(canary); err != nil {
		return fmt.Errorf("reconcileRouteTable failed: %w", err)
	}
	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (gmr *GlooMeshRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	_, primaryName, canaryName := canary.GetServiceNames()
	_, rtDelegate, err := gmr.getCanaryRouteTables(canary)

	if err != nil {
		return 0, 0, false, err
	}

	weightedRoute := getWeightedRoute(rtDelegate.Spec.Http)

	if weightedRoute == nil || weightedRoute.ForwardTo == nil {
		err = fmt.Errorf("RouteTable referenced with %s.%s doesn't have a route named 'weighted-route', which is required; or lacks a forward to action",
			rtDelegate.Name, canary.Namespace)
		return
	}

	for _, destination := range weightedRoute.ForwardTo.Destinations {
		if destination.Ref.Name == primaryName {
			primaryWeight = int(destination.Weight)
		}
		if destination.Ref.Name == canaryName {
			canaryWeight = int(destination.Weight)
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("RouteTable %s.%s does not contain routes for %s and %s",
			rtDelegate.Name, canary.Namespace, primaryName, canaryName)
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (gmr *GlooMeshRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	_, primaryName, canaryName := canary.GetServiceNames()
	_, rtDelegate, err := gmr.getCanaryRouteTables(canary)

	if err != nil {
		return err
	}

	weightedRoute := getWeightedRoute(rtDelegate.Spec.Http)

	if weightedRoute == nil {
		return fmt.Errorf("RouteTable referenced with %s.%s doesn't have a route named 'weighted-route', which is a requirement",
			rtDelegate.Name, rtDelegate.Namespace)
	}

	weightedRoute.ForwardTo = &v2.ForwardToAction{
		Destinations: []*v2.DestinationReference{
			makeRouteTableDestination(canary, primaryName, uint32(primaryWeight)),
			makeRouteTableDestination(canary, canaryName, uint32(canaryWeight)),
		},
	}

	_, err = gmr.flaggerClient.GloomeshnetworkingV2().RouteTables(rtDelegate.Namespace).Update(context.TODO(), rtDelegate,
		metav1.UpdateOptions{})

	if err != nil {
		return fmt.Errorf("RouteTable %s.%s update failed: %w", rtDelegate.Name, rtDelegate.Namespace, err)
	}

	return nil
}

func (gmr *GlooMeshRouter) Finalize(canary *flaggerv1.Canary) error {
	_, rtDelegate, err := gmr.getCanaryRouteTables(canary)
	if err != nil {
		return err
	}

	return gmr.flaggerClient.GloomeshnetworkingV2().RouteTables(rtDelegate.Namespace).Delete(context.TODO(), rtDelegate.Name,
		metav1.DeleteOptions{})
}

func (gmr *GlooMeshRouter) reconcileRouteTable(canary *flaggerv1.Canary) error {
	_, primaryName, canaryName := canary.GetServiceNames()

	if canary.Spec.RouteTableRef == nil {
		return fmt.Errorf("canary %s.%s doesn't define a routeTableRef, required when using the 'gloo-mesh' provider",
			canary.Name, canary.Namespace)
	}

	// Get delegator route table using the canary route table ref
	rtDelegator, rtDelegate, err := gmr.getCanaryRouteTables(canary)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("RouteTable referenced with %s.%s get query error %v",
			canary.Spec.RouteTableRef.Name, canary.Spec.RouteTableRef.Namespace, err)
	}

	// Define the initial RouteTable.Spec for the delegated route table
	rtDelegateSpec := v2.RouteTableSpec{
		WorkloadSelectors: rtDelegator.Spec.WorkloadSelectors,
		Weight:            rtDelegator.Spec.Weight,
		Http: []*v2.HTTPRoute{
			{
				Name: WeightedRouteName,
				ForwardTo: &v2.ForwardToAction{
					Destinations: []*v2.DestinationReference{
						makeRouteTableDestination(canary, primaryName, uint32(100)),
						makeRouteTableDestination(canary, canaryName, uint32(0)),
					},
				},
			},
		},
	}

	// create route table delegate if it doesn't exist
	if errors.IsNotFound(err) {
		return gmr.createRouteTableDelegate(canary, rtDelegator, rtDelegateSpec)
	}

	// Update existing route table delegate if necessary
	if rtDelegate != nil {
		err = gmr.updateRouteTableDelegate(rtDelegateSpec, rtDelegate, rtDelegator, err)
		if err != nil {
			return err
		}
		gmr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Delegate RouteTable %s.%s updated", rtDelegator.Name, rtDelegator.Namespace)
	}
	return nil
}

func (gmr *GlooMeshRouter) updateRouteTableDelegate(rtDelegateSpec v2.RouteTableSpec, rtDelegate *v2.RouteTable, rtDelegator *v2.RouteTable, err error) error {
	if diff := cmp.Diff(rtDelegateSpec,
		rtDelegate.Spec,
		cmpopts.IgnoreFields(v2.DestinationReference{}, "Weight"),
	); diff != "" {
		rtClone := rtDelegator.DeepCopy()
		rtClone.Spec = rtDelegateSpec
		rtClone.Name = rtDelegate.Name

		//If annotation kubectl.kubernetes.io/last-applied-configuration is present no need to duplicate
		//serialization.  If not present store the serialized object in annotation
		//flagger.kubernetes.app/original-configuration
		if _, ok := rtClone.Annotations[kubectlAnnotation]; !ok {
			b, err := json.Marshal(rtDelegator.Spec)
			if err != nil {
				gmr.logger.Warnf("Unable to marshal RouteTable %s for orig-configuration annotation", rtDelegator.Name)
			}

			if rtClone.ObjectMeta.Annotations == nil {
				rtClone.ObjectMeta.Annotations = make(map[string]string)
			} else {
				rtClone.ObjectMeta.Annotations = filterMetadata(rtClone.ObjectMeta.Annotations)
			}

			rtClone.ObjectMeta.Annotations[configAnnotation] = string(b)
		}

		_, err = gmr.flaggerClient.GloomeshnetworkingV2().RouteTables(rtDelegator.Namespace).Update(context.TODO(), rtClone, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("RouteTable %s.%s update error: %w", rtDelegate.Name, rtDelegate.Namespace, err)
		}
	}
	return nil
}

func (gmr *GlooMeshRouter) createRouteTableDelegate(canary *flaggerv1.Canary, rtDelegator *v2.RouteTable, rtDelegateSpec v2.RouteTableSpec) error {
	delegateName, err := getDelegatedRouteTableName(rtDelegator)

	if err != nil {
		return err
	}

	metadata := &metav1.ObjectMeta{}

	metadata.Labels = rtDelegator.Labels
	metadata.Annotations = rtDelegator.Annotations

	if rtDelegator.Labels == nil {
		metadata.Labels = make(map[string]string)
	}
	if rtDelegator.Annotations == nil {
		metadata.Annotations = make(map[string]string)
	}

	routetable := &v2.RouteTable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        delegateName,
			Namespace:   canary.Spec.RouteTableRef.Namespace,
			Labels:      metadata.Labels,
			Annotations: metadata.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			},
		},
		Spec: rtDelegateSpec,
	}

	_, err = gmr.flaggerClient.GloomeshnetworkingV2().RouteTables(canary.Namespace).Create(context.TODO(), routetable, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("RouteTable %s.%s create error: %w", routetable.Name, routetable.Namespace, err)
	}
	gmr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
		Infof("RouteTable %s.%s created", routetable.Name, routetable.Namespace)
	return nil
}

func getWeightedRoute(http []*v2.HTTPRoute) *v2.HTTPRoute {
	for _, route := range http {
		if route.Name == WeightedRouteName {
			return route
		}
	}
	return nil
}

// makeRouteTableDestination returns a reference to a destination
func makeRouteTableDestination(canary *flaggerv1.Canary, destinationName string, weight uint32) *v2.DestinationReference {

	dest := &v2.DestinationReference{
		Ref: &v2.ObjectReference{
			Name:      destinationName,
			Namespace: canary.Namespace,
		},
		Weight: weight,
	}

	// If portname specified then select by portname otherwise by port number
	if len(canary.Spec.Service.PortName) > 0 {
		dest.Port.Name = canary.Spec.Service.PortName
	} else {
		dest.Port = &v2.PortSelector{
			Number: uint32(canary.Spec.Service.Port),
		}
	}

	return dest
}

func getDelegatedRouteTableName(delegator *v2.RouteTable) (string, error) {
	for _, route := range delegator.Spec.Http {
		if route.Delegate != nil && len(route.Delegate.RouteTables) > 0 {
			return route.Delegate.RouteTables[0].Name, nil
		}
	}
	return "", fmt.Errorf("delegating route table %s.%s has no definition of delegate routes",
		delegator.Name, delegator.Namespace)
}

// getCanaryRouteTables - A cannary has a reference to a route table that we'll call the delegator.
// The delegator delegates decision making to another route table - the delegate.
// the method returns the delegator and the delegate, and any error that might occur. The delegate may be nil.
func (gmr *GlooMeshRouter) getCanaryRouteTables(canary *flaggerv1.Canary) (*v2.RouteTable, *v2.RouteTable, error) {
	rtDelegator, err := gmr.flaggerClient.GloomeshnetworkingV2().RouteTables(canary.Spec.RouteTableRef.Namespace).
		Get(context.TODO(), canary.Spec.RouteTableRef.Name, metav1.GetOptions{})

	if err != nil {
		err = fmt.Errorf("delegating Route Table %s.%s not found: %w", canary.Spec.RouteTableRef.Name,
			canary.Spec.RouteTableRef.Namespace, err)
		return nil, nil, err
	}

	delegateName, err := getDelegatedRouteTableName(rtDelegator)

	rtDelegate, err := gmr.flaggerClient.GloomeshnetworkingV2().RouteTables(rtDelegator.Namespace).
		Get(context.TODO(), delegateName, metav1.GetOptions{})

	// If the delegate route table is not found, then we'll create it later on.
	if err != nil {
		return rtDelegator, rtDelegate, err
	}

	return rtDelegator, rtDelegate, nil
}
