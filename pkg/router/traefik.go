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

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	traefikv1alpha1 "github.com/fluxcd/flagger/pkg/apis/traefik/v1alpha1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TraefikRouter is managing Traefik service
type TraefikRouter struct {
	traefikClient clientset.Interface
	logger        *zap.SugaredLogger
	setOwnerRefs  bool
}

// Reconcile creates or updates the Traefik service
func (tr *TraefikRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	newSpec := traefikv1alpha1.ServiceSpec{
		Weighted: &traefikv1alpha1.WeightedRoundRobin{
			Services: []traefikv1alpha1.Service{
				{
					Name:      primaryName,
					Namespace: canary.Namespace,
					Port:      canary.Spec.Service.Port,
					Weight:    100,
				},
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

	traefikService, err := tr.traefikClient.TraefikV1alpha1().TraefikServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		traefikService = &traefikv1alpha1.TraefikService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        apexName,
				Namespace:   canary.Namespace,
				Labels:      newMetadata.Labels,
				Annotations: newMetadata.Annotations,
			},
			Spec: newSpec,
		}
		if tr.setOwnerRefs {
			traefikService.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(canary, schema.GroupVersionKind{
					Group:   flaggerv1.SchemeGroupVersion.Group,
					Version: flaggerv1.SchemeGroupVersion.Version,
					Kind:    flaggerv1.CanaryKind,
				}),
			}
		}

		_, err = tr.traefikClient.TraefikV1alpha1().TraefikServices(canary.Namespace).Create(context.TODO(), traefikService, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("TraefikService %s.%s create error: %w", apexName, canary.Namespace, err)
		}
		tr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TraefikService %s.%s created", traefikService.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("TraefikService %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	// update TraefikService but keep the original service weights
	if traefikService != nil {
		if len(traefikService.Spec.Weighted.Services) == 2 {
			newSpec.Weighted.Services = append(
				newSpec.Weighted.Services,
				traefikv1alpha1.Service{
					Name:      canaryName,
					Namespace: canary.Namespace,
					Port:      canary.Spec.Service.Port,
					Weight:    100,
				},
			)
		}

		specDiff := cmp.Diff(
			newSpec,
			traefikService.Spec,
			cmpopts.IgnoreFields(traefikv1alpha1.Service{}, "Weight"),
		)
		labelsDiff := cmp.Diff(newMetadata.Labels, traefikService.Labels, cmpopts.EquateEmpty())
		annotationsDiff := cmp.Diff(newMetadata.Annotations, traefikService.Annotations, cmpopts.EquateEmpty())
		if specDiff != "" || labelsDiff != "" || annotationsDiff != "" {
			clone := traefikService.DeepCopy()
			clone.Spec = newSpec
			clone.ObjectMeta.Annotations = newMetadata.Annotations
			clone.ObjectMeta.Labels = newMetadata.Labels

			_, err = tr.traefikClient.TraefikV1alpha1().TraefikServices(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("TraefikService %s.%s update error: %w", apexName, canary.Namespace, err)
			}
			tr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("TraefikService %s.%s updated", traefikService.GetName(), canary.Namespace)
		}
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (tr *TraefikRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, _ := canary.GetServiceNames()

	traefikService, err := tr.traefikClient.TraefikV1alpha1().TraefikServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("TraefikService %s.%s query error: %w", apexName, canary.Namespace, err)
		return
	}

	if len(traefikService.Spec.Weighted.Services) < 1 {
		err = fmt.Errorf("TraefikService %s.%s services not found", apexName, canary.Namespace)
		return
	}

	for _, s := range traefikService.Spec.Weighted.Services {
		if s.Name == primaryName {
			primaryWeight = int(s.Weight)
			canaryWeight = 100 - primaryWeight
			return

		}
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (tr *TraefikRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	if primaryWeight == 0 && canaryWeight == 0 {
		return fmt.Errorf("RoutingRule %s.%s update failed: no valid weights", apexName, canary.Namespace)
	}
	traefikService, err := tr.traefikClient.TraefikV1alpha1().TraefikServices(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("TraefikService %s.%s query error: %w", apexName, canary.Namespace, err)
	}

	services := []traefikv1alpha1.Service{
		{
			Name:      primaryName,
			Namespace: canary.Namespace,
			Port:      canary.Spec.Service.Port,
			Weight:    uint(primaryWeight),
		},
	}
	if canaryWeight > 0 {
		services = append(services, traefikv1alpha1.Service{
			Name:      canaryName,
			Namespace: canary.Namespace,
			Port:      canary.Spec.Service.Port,
			Weight:    uint(canaryWeight),
		})
	}

	traefikService.Spec.Weighted.Services = services

	_, err = tr.traefikClient.TraefikV1alpha1().TraefikServices(canary.Namespace).Update(context.TODO(), traefikService, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("TraefikService %s.%s update error: %w", apexName, canary.Namespace, err)
	}
	return nil
}

func (tr *TraefikRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
