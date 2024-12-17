package canary

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	http "github.com/fluxcd/flagger/pkg/apis/http/v1alpha1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HTTPScaledObjectReconciler is a ScalerReconciler that reconciles KEDA HTTPScaledObjects.
type HTTPScaledObjectReconciler struct {
	kubeClient         kubernetes.Interface
	flaggerClient      clientset.Interface
	logger             *zap.SugaredLogger
	includeLabelPrefix []string
}

func (httpsor *HTTPScaledObjectReconciler) ReconcilePrimaryScaler(cd *flaggerv1.Canary, init bool) error {
	if cd.Spec.AutoscalerRef != nil {
		if err := httpsor.reconcilePrimaryScaler(cd, init); err != nil {
			return err
		}
	}
	return nil
}

func (httpsor *HTTPScaledObjectReconciler) reconcilePrimaryScaler(cd *flaggerv1.Canary, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	targetSo, err := httpsor.flaggerClient.HttpV1alpha1().HTTPScaledObjects(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Keda HTTPScaledObject %s.%s get query error: %w",
			cd.Spec.AutoscalerRef.Name, cd.Namespace, err)
	}
	targetSoClone := targetSo.DeepCopy()
	primaryServiceName := fmt.Sprintf("%s-primary", targetSoClone.Spec.ScaleTargetRef.Service)

	httpsoSpec := http.HTTPScaledObjectSpec{
		Hosts:        targetSoClone.Spec.Hosts,
		PathPrefixes: targetSoClone.Spec.PathPrefixes,
		ScaleTargetRef: http.ScaleTargetRef{
			Name:       primaryName,
			APIVersion: targetSoClone.Spec.ScaleTargetRef.APIVersion,
			Kind:       targetSoClone.Spec.ScaleTargetRef.Kind,
			Service:    primaryServiceName,
			Port:       targetSoClone.Spec.ScaleTargetRef.Port,
		},
		Replicas:              targetSoClone.Spec.Replicas,
		TargetPendingRequests: targetSoClone.Spec.TargetPendingRequests,
		CooldownPeriod:        targetSoClone.Spec.CooldownPeriod,
		ScalingMetric:         targetSoClone.Spec.ScalingMetric,
	}

	if scalingSet := cd.Spec.AutoscalerRef.PrimaryScalingSet; scalingSet != nil {
		httpsoSpec.ScalingSet = &http.HTTPSalingSetTargetRef{
			Name: scalingSet.Name,
			Kind: scalingSet.Kind,
		}
	}

	if replicas := cd.Spec.AutoscalerRef.PrimaryScalerReplicas; replicas != nil {
		if minReplicas := replicas.MinReplicas; minReplicas != nil {
			httpsoSpec.Replicas.Min = minReplicas
		}
		if maxReplicas := replicas.MaxReplicas; maxReplicas != nil {
			httpsoSpec.Replicas.Max = maxReplicas
		}
	}

	primarySoName := fmt.Sprintf("%s-primary", cd.Spec.AutoscalerRef.Name)
	primarySo, err := httpsor.flaggerClient.HttpV1alpha1().HTTPScaledObjects(cd.Namespace).Get(context.TODO(), primarySoName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		primarySo = &http.HTTPScaledObject{
			ObjectMeta: makeObjectMeta(primarySoName, targetSoClone.Labels, cd),
			Spec:       httpsoSpec,
		}
		_, err = httpsor.flaggerClient.HttpV1alpha1().HTTPScaledObjects(cd.Namespace).Create(context.TODO(), primarySo, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating Keda HTTPScaledObject %s.%s failed: %w",
				primarySo.Name, primarySo.Namespace, err)
		}
		httpsor.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof(
			"Keda HTTPScaledObject %s.%s created", primarySo.GetName(), cd.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("Keda HTTPScaledObject %s.%s get query failed: %w",
			primarySo.Name, primarySo.Namespace, err)
	}

	if primarySo != nil && !init {
		if diff := cmp.Diff(httpsoSpec, primarySo.Spec); diff != "" {
			err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				primarySo, err := httpsor.flaggerClient.HttpV1alpha1().HTTPScaledObjects(cd.Namespace).Get(context.TODO(), primarySoName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				primarySoClone := primarySo.DeepCopy()
				primarySoClone.Spec = httpsoSpec

				filteredAnnotations := includeLabelsByPrefix(primarySo.Annotations, httpsor.includeLabelPrefix)
				primarySoClone.Annotations = filteredAnnotations
				filteredLabels := includeLabelsByPrefix(primarySo.ObjectMeta.Labels, httpsor.includeLabelPrefix)
				primarySoClone.Labels = filteredLabels

				_, err = httpsor.flaggerClient.HttpV1alpha1().HTTPScaledObjects(cd.Namespace).Update(context.TODO(), primarySoClone, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				return fmt.Errorf("updating HTTPScaledObject %s.%s failed: %w", primarySoName, cd.Namespace, err)
			}
		}
	}
	return nil
}

func (httpsor *HTTPScaledObjectReconciler) PauseTargetScaler(cd *flaggerv1.Canary) error {
	return nil
}

func (httpsor *HTTPScaledObjectReconciler) ResumeTargetScaler(cd *flaggerv1.Canary) error {
	return nil
}
