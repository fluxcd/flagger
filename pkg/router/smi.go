package router

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	smiv1 "github.com/weaveworks/flagger/pkg/apis/smi/v1alpha1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
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
	targetName := canary.Spec.TargetRef.Name
	canaryName := fmt.Sprintf("%s-canary", targetName)
	primaryName := fmt.Sprintf("%s-primary", targetName)

	var host string
	if len(canary.Spec.Service.Hosts) > 0 {
		host = canary.Spec.Service.Hosts[0]
	} else {
		host = targetName
	}

	tsSpec := smiv1.TrafficSplitSpec{
		Service: host,
		Backends: []smiv1.TrafficSplitBackend{
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

	ts, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Get(targetName, metav1.GetOptions{})
	// create traffic split
	if errors.IsNotFound(err) {
		t := &smiv1.TrafficSplit{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
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

		_, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Create(t)
		if err != nil {
			return err
		}

		sr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficSplit %s.%s created", t.GetName(), canary.Namespace)
		return nil
	}

	if err != nil {
		return fmt.Errorf("traffic split %s query error %v", targetName, err)
	}

	// update traffic split
	if diff := cmp.Diff(tsSpec, ts.Spec, cmpopts.IgnoreTypes(resource.Quantity{})); diff != "" {
		tsClone := ts.DeepCopy()
		tsClone.Spec = tsSpec

		_, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Update(tsClone)
		if err != nil {
			return fmt.Errorf("TrafficSplit %s update error %v", targetName, err)
		}

		sr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficSplit %s.%s updated", targetName, canary.Namespace)
		return nil
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (sr *SmiRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	err error,
) {
	targetName := canary.Spec.TargetRef.Name
	canaryName := fmt.Sprintf("%s-canary", targetName)
	primaryName := fmt.Sprintf("%s-primary", targetName)
	ts, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			err = fmt.Errorf("TrafficSplit %s.%s not found", targetName, canary.Namespace)
			return
		}
		err = fmt.Errorf("TrafficSplit %s.%s query error %v", targetName, canary.Namespace, err)
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
			targetName, canary.Namespace, primaryName, canaryName)
	}

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (sr *SmiRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
) error {
	targetName := canary.Spec.TargetRef.Name
	canaryName := fmt.Sprintf("%s-canary", targetName)
	primaryName := fmt.Sprintf("%s-primary", targetName)
	ts, err := sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("TrafficSplit %s.%s not found", targetName, canary.Namespace)

		}
		return fmt.Errorf("TrafficSplit %s.%s query error %v", targetName, canary.Namespace, err)
	}

	backends := []smiv1.TrafficSplitBackend{
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

	_, err = sr.smiClient.SplitV1alpha1().TrafficSplits(canary.Namespace).Update(tsClone)
	if err != nil {
		return fmt.Errorf("TrafficSplit %s update error %v", targetName, err)
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
