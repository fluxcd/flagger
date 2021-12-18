package router

import (
	"context"
	"fmt"
	"strings"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	kumav1alpha1 "github.com/fluxcd/flagger/pkg/apis/kuma/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// KumaRouter is managing TrafficRoute objects
type KumaRouter struct {
	kubeClient    kubernetes.Interface
	kumaClient    clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Reconcile creates or updates the Kuma TrafficRoute
func (kr *KumaRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	trSpec := kumav1alpha1.TrafficRouteSpec{
		Sources: []*kumav1alpha1.Selector{
			{
				Match: map[string]string{
					"kuma.io/service": "*",
				},
			},
		},
		Destinations: []*kumav1alpha1.Selector{
			{
				Match: map[string]string{
					"kuma.io/service": fmt.Sprintf("%s_%s_svc_%d", apexName, canary.Namespace, canary.Spec.Service.Port),
				},
			},
		},
		Conf: &kumav1alpha1.TrafficRouteConf{
			Split: []*kumav1alpha1.TrafficRouteSplit{
				{
					Weight: uint32(100),
					Destination: map[string]string{
						"kuma.io/service": fmt.Sprintf("%s_%s_svc_%d", primaryName, canary.Namespace, canary.Spec.Service.Port),
					},
				},
				{
					Weight: uint32(0),
					Destination: map[string]string{
						"kuma.io/service": fmt.Sprintf("%s_%s_svc_%d", canaryName, canary.Namespace, canary.Spec.Service.Port),
					},
				},
			},
		},
	}

	tr, err := kr.kumaClient.KumaV1alpha1().TrafficRoutes().Get(context.TODO(), apexName, metav1.GetOptions{})

	// create TrafficRoute
	if errors.IsNotFound(err) {
		metadata := canary.Spec.Service.Apex
		if metadata == nil {
			metadata = &flaggerv1.CustomMetadata{}
		}
		if metadata.Labels == nil {
			metadata.Labels = make(map[string]string)
		}
		if metadata.Annotations == nil {
			metadata.Annotations = make(map[string]string)
			metadata.Annotations[fmt.Sprintf("%d.service.kuma.io", canary.Spec.Service.Port)] = "http"
		}

		meshName, ok := canary.Annotations["kuma.io/mesh"]
		if !ok {
			meshName = "default"
		}

		t := &kumav1alpha1.TrafficRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: apexName,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
				Annotations: filterMetadata(metadata.Annotations),
			},
			Spec: trSpec,
			Mesh: meshName,
		}

		_, err := kr.kumaClient.KumaV1alpha1().TrafficRoutes().Create(context.TODO(), t, metav1.CreateOptions{})

		if err != nil {
			return fmt.Errorf("TrafficRoute %s create error: %w", apexName, err)
		}

		kr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficRoute %s created", t.GetName())
		return nil
	} else if err != nil {
		return fmt.Errorf("TrafficRoute %s get query error: %w", apexName, err)
	}

	// update TrafficRoute
	if diff := cmp.Diff(trSpec, tr.Spec, cmpopts.IgnoreFields(kumav1alpha1.TrafficRouteSplit{}, "Weight")); diff != "" {
		trClone := tr.DeepCopy()
		trClone.Spec = trSpec

		_, err := kr.kumaClient.KumaV1alpha1().TrafficRoutes().Update(context.TODO(), trClone, metav1.UpdateOptions{})

		if err != nil {
			return fmt.Errorf("TrafficRoute %s update error: %w", apexName, err)
		}

		kr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("TrafficRoute %s.%s updated", apexName, canary.Namespace)
		return nil
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (kr *KumaRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	tr, err := kr.kumaClient.KumaV1alpha1().TrafficRoutes().Get(context.TODO(), apexName, metav1.GetOptions{})

	if err != nil {
		err = fmt.Errorf("TrafficRoute %s get query error %v", apexName, err)
		return
	}

	for _, split := range tr.Spec.Conf.Split {
		if strings.Split(split.Destination["kuma.io/service"], "_")[0] == primaryName {
			primaryWeight = int(split.Weight)
			canaryWeight = 100 - primaryWeight
		}
	}

	if primaryWeight == 0 && canaryWeight == 0 {
		err = fmt.Errorf("TrafficRoute %s does not contain routes for %s and %s",
			apexName, primaryName, canaryName)
	}

	mirrored = false

	return
}

// SetRoutes updates the destinations weight for primary and canary
func (kr *KumaRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()
	tr, err := kr.kumaClient.KumaV1alpha1().TrafficRoutes().Get(context.TODO(), apexName, metav1.GetOptions{})

	if err != nil {
		return fmt.Errorf("TrafficRoute %s get query error %v", apexName, err)
	}

	conf := &kumav1alpha1.TrafficRouteConf{
		Split: []*kumav1alpha1.TrafficRouteSplit{
			{
				Weight: uint32(primaryWeight),
				Destination: map[string]string{
					"kuma.io/service": fmt.Sprintf("%s_%s_svc_%d", primaryName, canary.Namespace, canary.Spec.Service.Port),
				},
			},
			{
				Weight: uint32(canaryWeight),
				Destination: map[string]string{
					"kuma.io/service": fmt.Sprintf("%s_%s_svc_%d", canaryName, canary.Namespace, canary.Spec.Service.Port),
				},
			},
		},
	}

	trClone := tr.DeepCopy()
	trClone.Spec.Conf = conf

	_, err = kr.kumaClient.KumaV1alpha1().TrafficRoutes().Update(context.TODO(), trClone, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("TrafficRoute %s update error %v", apexName, err)
	}

	return nil
}

func (kr *KumaRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
