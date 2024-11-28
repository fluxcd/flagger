package canary

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	keda "github.com/fluxcd/flagger/pkg/apis/keda/v1alpha1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ScaledObjectReconciler is a ScalerReconciler that reconciles KEDA ScaledObjects.
type ScaledObjectReconciler struct {
	kubeClient         kubernetes.Interface
	flaggerClient      clientset.Interface
	logger             *zap.SugaredLogger
	includeLabelPrefix []string
}

func (sor *ScaledObjectReconciler) ReconcilePrimaryScaler(cd *flaggerv1.Canary, init bool) error {
	if cd.Spec.AutoscalerRef != nil {
		if err := sor.reconcilePrimaryScaler(cd, init); err != nil {
			return err
		}
	}
	return nil
}

func (sor *ScaledObjectReconciler) reconcilePrimaryScaler(cd *flaggerv1.Canary, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	targetSo, err := sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Keda ScaledObject %s.%s get query error: %w",
			cd.Spec.AutoscalerRef.Name, cd.Namespace, err)
	}
	targetSoClone := targetSo.DeepCopy()

	setPrimaryScaledObjectQueries(cd, targetSoClone.Spec.Triggers)

	setPrimaryScaledObjectHPA(targetSoClone)

	soSpec := keda.ScaledObjectSpec{
		ScaleTargetRef: &keda.ScaleTarget{
			Name:                   primaryName,
			Kind:                   targetSoClone.Spec.ScaleTargetRef.Kind,
			APIVersion:             targetSoClone.Spec.ScaleTargetRef.APIVersion,
			EnvSourceContainerName: targetSoClone.Spec.ScaleTargetRef.EnvSourceContainerName,
		},
		PollingInterval:  targetSoClone.Spec.PollingInterval,
		CooldownPeriod:   targetSoClone.Spec.CooldownPeriod,
		MinReplicaCount:  targetSoClone.Spec.MinReplicaCount,
		MaxReplicaCount:  targetSoClone.Spec.MaxReplicaCount,
		Advanced:         targetSoClone.Spec.Advanced,
		Triggers:         targetSoClone.Spec.Triggers,
		Fallback:         targetSoClone.Spec.Fallback,
		IdleReplicaCount: targetSoClone.Spec.IdleReplicaCount,
	}

	if replicas := cd.Spec.AutoscalerRef.PrimaryScalerReplicas; replicas != nil {
		if minReplicas := replicas.MinReplicas; minReplicas != nil {
			soSpec.MinReplicaCount = minReplicas
		}
		if maxReplicas := replicas.MaxReplicas; maxReplicas != nil {
			soSpec.MaxReplicaCount = maxReplicas
		}
	}

	primarySoName := fmt.Sprintf("%s-primary", cd.Spec.AutoscalerRef.Name)
	primarySo, err := sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Get(context.TODO(), primarySoName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		primarySo = &keda.ScaledObject{
			// Passing in the annotations from the targetSo so that they are carried over to the primarySo. This is required so that the transfer ownership annotation can be added.
			ObjectMeta: makeObjectMetaSo(primarySoName, targetSoClone.Labels, targetSoClone.Annotations, cd),
			Spec:       soSpec,
		}
		_, err = sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Create(context.TODO(), primarySo, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating Keda ScaledObject %s.%s failed: %w",
				primarySo.Name, primarySo.Namespace, err)
		}
		sor.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof(
			"Keda ScaledObject %s.%s created", primarySo.GetName(), cd.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("Keda ScaledObject %s.%s get query failed: %w",
			primarySo.Name, primarySo.Namespace, err)
	}

	if primarySo != nil && !init {
		if diff := cmp.Diff(soSpec, primarySo.Spec); diff != "" {
			err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				primarySo, err := sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Get(context.TODO(), primarySoName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				primarySoClone := primarySo.DeepCopy()
				primarySoClone.Spec = soSpec

				filteredAnnotations := includeLabelsByPrefix(primarySo.Annotations, sor.includeLabelPrefix)
				primarySoClone.Annotations = filteredAnnotations
				filteredLabels := includeLabelsByPrefix(primarySo.ObjectMeta.Labels, sor.includeLabelPrefix)
				primarySoClone.Labels = filteredLabels

				_, err = sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Update(context.TODO(), primarySoClone, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				return fmt.Errorf("updating ScaledObject %s.%s failed: %w", primarySoName, cd.Namespace, err)
			}
		}
	}
	return nil
}

func (sor *ScaledObjectReconciler) PauseTargetScaler(cd *flaggerv1.Canary) error {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		so, err := sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("Keda ScaledObject %s.%s get query error: %w",
				cd.Spec.AutoscalerRef.Name, cd.Namespace, err)
		}
		soClone := so.DeepCopy()

		if soClone.ObjectMeta.Annotations == nil {
			soClone.ObjectMeta.Annotations = make(map[string]string)
		}
		soClone.ObjectMeta.Annotations[keda.PausedReplicasAnnotation] = "0"

		_, err = sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Update(context.TODO(), soClone, metav1.UpdateOptions{})
		return err
	})

	return err
}

func (sor *ScaledObjectReconciler) ResumeTargetScaler(cd *flaggerv1.Canary) error {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		so, err := sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("Keda ScaledObject %s.%s get query error: %w",
				cd.Spec.AutoscalerRef.Name, cd.Namespace, err)
		}
		soClone := so.DeepCopy()

		if soClone.ObjectMeta.Annotations != nil {
			if _, ok := soClone.ObjectMeta.Annotations[keda.PausedReplicasAnnotation]; ok {
				delete(soClone.Annotations, keda.PausedReplicasAnnotation)
			}
		}

		_, err = sor.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Update(context.TODO(), soClone, metav1.UpdateOptions{})
		return err
	})

	return err
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq() string {
	rand.Seed(time.Now().UnixNano())

	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// setPrimaryScaledObjectQueries accepts a list of ScaleTriggers and modifies the query
// for each of them.
func setPrimaryScaledObjectQueries(cd *flaggerv1.Canary, triggers []keda.ScaleTriggers) {
	for _, trigger := range triggers {
		if cd.Spec.AutoscalerRef.PrimaryScalerQueries != nil {
			// If .spec.autoscalerRef.primaryScalerQueries is specified, the triggers must be named,
			// otherwise it might lead to unexpected behaviour.
			for name, query := range cd.Spec.AutoscalerRef.PrimaryScalerQueries {
				if trigger.Name == name {
					trigger.Metadata["query"] = query
				}
			}
		} else {
			for key, val := range trigger.Metadata {
				if key == "query" {
					// We could've used regex with negative look-arounds to avoid using a placeholder, but Go does
					// not support them. We need them because, we need to replace both "podinfo" and "podinfo-canary"
					// (assuming "podinfo" to be the targetRef name), with "podinfo-primary". This placeholder makes
					// sure that we don't end up with a query which contains terms like "podinfo-primary-canary" or
					// "podinfo-primary-primary". This is a best effort approach, and users should be encouraged to
					// check the generated query and opt for using `autoscalerRef.primaryScalerQuery` if the former
					// doesn't look correct.
					placeholder := randSeq()
					replaced := strings.ReplaceAll(val, fmt.Sprintf("%s-canary", cd.Spec.TargetRef.Name), placeholder)
					replaced = strings.ReplaceAll(replaced, cd.Spec.TargetRef.Name, fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name))
					replaced = strings.ReplaceAll(replaced, placeholder, fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name))
					trigger.Metadata[key] = replaced
				}
			}
		}
	}
}

func makeObjectMetaSo(name string, labels map[string]string, annotations map[string]string, cd *flaggerv1.Canary) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        name,
		Namespace:   cd.Namespace,
		Labels:      filterMetadata(labels),
		Annotations: filterMetadata(annotations),
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(cd, schema.GroupVersionKind{
				Group:   flaggerv1.SchemeGroupVersion.Group,
				Version: flaggerv1.SchemeGroupVersion.Version,
				Kind:    flaggerv1.CanaryKind,
			}),
		},
	}
}

func setPrimaryScaledObjectHPA(targetSoClone *keda.ScaledObject) {
	if targetSoClone.Spec.Advanced == nil {
		targetSoClone.Spec.Advanced = &keda.AdvancedConfig{}
	}
	if targetSoClone.Spec.Advanced.HorizontalPodAutoscalerConfig == nil {
		targetSoClone.Spec.Advanced.HorizontalPodAutoscalerConfig = &keda.HorizontalPodAutoscalerConfig{}
	}
	if targetSoClone.Spec.Advanced.HorizontalPodAutoscalerConfig.Name != "" {
		// if the target scaled object has the hpa name set, then append "-primary" to the primary scaled object hpa name
		// if the target scaled object does not have the hpa name set, then it will use the default set by keda
		targetSoClone.Spec.Advanced.HorizontalPodAutoscalerConfig.Name = fmt.Sprintf("%s-primary", targetSoClone.Spec.Advanced.HorizontalPodAutoscalerConfig.Name)
	}
}
