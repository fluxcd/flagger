package router

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	smiv1alpha1 "github.com/weaveworks/flagger/pkg/apis/smi/v1alpha1"
	smiv1alpha2 "github.com/weaveworks/flagger/pkg/apis/smi/v1alpha2"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

type SmiRouter struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	smiClient     clientset.Interface
	logger        *zap.SugaredLogger
	targetMesh    string
}

// Reconcile creates or updates the SMI traffic split
func (sr *SmiRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	var host string
	if len(canary.Spec.Service.Hosts) > 0 {
		host = canary.Spec.Service.Hosts[0]
	} else {
		host = apexName
	}

	tsSpec := smiv1alpha1.TrafficSplitSpec{
		Service: host,
		Backends: []smiv1alpha1.TrafficSplitBackend{
			{
				Service: canaryName,
				Weight:  resource.NewQuantity(0, resource.DecimalExponent),
			},
			{
				Service: primaryName,
				Weight:  resource.NewQuantity(100, resource.DecimalExponent),
			},
		},
	}

	ts, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	// create traffic split
	if errors.IsNotFound(err) {
		t := &smiv1alpha1.TrafficSplit{
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

		_, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Create(context.TODO(), t, metav1.CreateOptions{})
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
	if diff := cmp.Diff(tsSpec, ts.Spec, cmpopts.IgnoreTypes(resource.Quantity{})); diff != "" {
		tsClone := ts.DeepCopy()
		tsClone.Spec = tsSpec

		_, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Update(context.TODO(), tsClone, metav1.UpdateOptions{})
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
func (sr *SmiRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	ts, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("TrafficSplit %s.%s get query error %v", apexName, canary.Namespace, err)
		return
	}

	for _, r := range ts.Spec.Backends {
		w, _ := r.Weight.AsInt64()
		if r.Service == primaryName {
			primaryWeight = int(w)
		}
		if r.Service == canaryName {
			canaryWeight = int(w)
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
func (sr *SmiRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	ts, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("TrafficSplit %s.%s get query error %v", apexName, canary.Namespace, err)
	}

	backends := []smiv1alpha1.TrafficSplitBackend{
		{
			Service: canaryName,
			Weight:  resource.NewQuantity(int64(canaryWeight), resource.DecimalExponent),
		},
		{
			Service: primaryName,
			Weight:  resource.NewQuantity(int64(primaryWeight), resource.DecimalExponent),
		},
	}

	tsClone := ts.DeepCopy()
	tsClone.Spec.Backends = backends

	_, err = sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Update(context.TODO(), tsClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("TrafficSplit %s.%s update error %v", apexName, canary.Namespace, err)
	}

	return nil
}

func (sr *SmiRouter) makeAnnotations(gateways []string) map[string]string {
	res := make(map[string]string)
	if sr.targetMesh == "istio" && len(gateways) > 0 {
		g, _ := json.Marshal(gateways)
		res["VirtualService.v1alpha3.networking.istio.io/spec.gateways"] = string(g)
	}
	return res
}

// getWithConvert overrides invalid traffic split and sets weight based on the canary status
func (sr *SmiRouter) getWithConvert(canary *flaggerv1.Canary, host string) (*smiv1alpha2.TrafficSplit, error) {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	ts, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if errors.IsInvalid(err) {
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
			Spec: smiv1alpha2.TrafficSplitSpec{
				Service: host,
				Backends: []smiv1alpha2.TrafficSplitBackend{
					{
						Service: canaryName,
						Weight:  canary.Status.CanaryWeight,
					},
					{
						Service: primaryName,
						Weight:  100 - canary.Status.CanaryWeight,
					},
				},
			},
		}

		_, err := sr.smiClient.SplitV1alpha2().TrafficSplits(canary.Namespace).Update(context.TODO(), t, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("TrafficSplit %s.%s update error: %w", apexName, canary.Namespace, err)
		}

		sr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficSplit %s.%s converted", t.GetName(), canary.Namespace)
	} else if err != nil {
		return nil, fmt.Errorf("TrafficSplit %s.%s get query error %v", apexName, canary.Namespace, err)
	}
	return ts, nil
}

func (sr *SmiRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
