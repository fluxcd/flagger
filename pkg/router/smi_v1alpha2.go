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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	smiv1alpha2 "github.com/fluxcd/flagger/pkg/apis/smi/v1alpha2"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

type Smiv1alpha2Router struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	smiClient     clientset.Interface
	logger        *zap.SugaredLogger
	targetMesh    string
}

// Reconcile creates or updates the SMI traffic split
func (sr *Smiv1alpha2Router) Reconcile(canary *flaggerv1.Canary) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	var host string
	if len(canary.Spec.Service.Hosts) > 0 {
		host = canary.Spec.Service.Hosts[0]
	} else {
		host = apexName
	}

	tsSpec := smiv1alpha2.TrafficSplitSpec{
		Service: host,
		Backends: []smiv1alpha2.TrafficSplitBackend{
			{
				Service: canaryName,
				Weight:  0,
			},
			{
				Service: primaryName,
				Weight:  100,
			},
		},
	}

	ts, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	// create traffic split
	if errors.IsNotFound(err) {
		t := &smiv1alpha2.TrafficSplit{
			ObjectMeta: metav1.ObjectMeta{
				Name:      apexName,
				Namespace: canary.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
				Annotations: sr.makeAnnotations(canary.Spec.Service.Gateways),
			},
			Spec: tsSpec,
		}

		_, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Create(context.TODO(), t, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("TrafficSplit %s.%s create error: %w", apexName, canary.Namespace, err)
		}

		sr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficSplit %s.%s created", t.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("TrafficSplit %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	// update traffic split
	if diff := cmp.Diff(tsSpec, ts.Spec, cmpopts.IgnoreFields(smiv1alpha2.TrafficSplitBackend{}, "Weight")); diff != "" {
		tsClone := ts.DeepCopy()
		tsClone.Spec = tsSpec

		_, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Update(context.TODO(), tsClone, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("TrafficSplit %s.%s update error: %w", apexName, canary.Namespace, err)
		}

		sr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficSplit %s.%s updated", apexName, canary.Namespace)
		return nil
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (sr *Smiv1alpha2Router) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	ts, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("TrafficSplit %s.%s get query error %v", apexName, canary.Namespace, err)
		return
	}

	for _, r := range ts.Spec.Backends {
		if r.Service == primaryName {
			primaryWeight = r.Weight
		}
		if r.Service == canaryName {
			canaryWeight = r.Weight
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("TrafficSplit %s.%s does not contain routes for %s and %s",
			apexName, canary.Namespace, primaryName, canaryName)
	}

	mirrored = false

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (sr *Smiv1alpha2Router) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	ts, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("TrafficSplit %s.%s get query error %v", apexName, canary.Namespace, err)
	}

	backends := []smiv1alpha2.TrafficSplitBackend{
		{
			Service: canaryName,
			Weight:  canaryWeight,
		},
		{
			Service: primaryName,
			Weight:  primaryWeight,
		},
	}

	tsClone := ts.DeepCopy()
	tsClone.Spec.Backends = backends

	_, err = sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Update(context.TODO(), tsClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("TrafficSplit %s.%s update error %v", apexName, canary.Namespace, err)
	}

	return nil
}

func (sr *Smiv1alpha2Router) makeAnnotations(gateways []string) map[string]string {
	res := make(map[string]string)
	if sr.targetMesh == "istio" && len(gateways) > 0 {
		g, _ := json.Marshal(gateways)
		res["VirtualService.v1alpha3.networking.istio.io/spec.gateways"] = string(g)
	}
	return res
}

func (sr *Smiv1alpha2Router) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
