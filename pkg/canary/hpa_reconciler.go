package canary

import (
	"context"
	"fmt"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	hpav2 "k8s.io/api/autoscaling/v2"
	hpav2beta2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// HPAReconciler is a ScalerReconciler that reconciles HPAs.
type HPAReconciler struct {
	kubeClient         kubernetes.Interface
	flaggerClient      clientset.Interface
	logger             *zap.SugaredLogger
	includeLabelPrefix []string
}

func (hr *HPAReconciler) ReconcilePrimaryScaler(cd *flaggerv1.Canary, init bool) error {
	if cd.Spec.AutoscalerRef != nil {
		if err := hr.reconcilePrimaryHpa(cd, init); err != nil {
			return err
		}
	}
	return nil
}

func (hr *HPAReconciler) reconcilePrimaryHpa(cd *flaggerv1.Canary, init bool) error {
	var betaHpa *hpav2beta2.HorizontalPodAutoscaler
	hpa, err := hr.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		hpa = nil
		hr.logger.Debugf("v2 HorizontalPodAutoscaler %s.%s get query error: %s; falling back to v2beta2",
			cd.Namespace, cd.Spec.AutoscalerRef.Name, err)
		var betaErr error
		betaHpa, betaErr = hr.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
		if betaErr != nil {
			return fmt.Errorf("HorizontalPodAutoscaler %s.%s get query error for both v2beta2: %s and v2: %s",
				cd.Spec.AutoscalerRef.Name, cd.Namespace, betaErr, err)
		}
	}

	if hpa != nil {
		if err = hr.reconcilePrimaryHpaV2(cd, hpa, init); err != nil {
			return err
		}
	} else if betaHpa != nil {
		if err = hr.reconcilePrimaryHpaV2Beta2(cd, betaHpa, init); err != nil {
			return err
		}
	}

	return nil
}

func (hr *HPAReconciler) reconcilePrimaryHpaV2(cd *flaggerv1.Canary, hpa *hpav2.HorizontalPodAutoscaler, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	hpaSpec := hpav2.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: hpav2.CrossVersionObjectReference{
			Name:       primaryName,
			Kind:       hpa.Spec.ScaleTargetRef.Kind,
			APIVersion: hpa.Spec.ScaleTargetRef.APIVersion,
		},
		MinReplicas: hpa.Spec.MinReplicas,
		MaxReplicas: hpa.Spec.MaxReplicas,
		Metrics:     hpa.Spec.Metrics,
		Behavior:    hpa.Spec.Behavior,
	}

	primaryHpaName := fmt.Sprintf("%s-primary", cd.Spec.AutoscalerRef.Name)
	primaryHpa, err := hr.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})

	// create HPA
	if errors.IsNotFound(err) {
		primaryHpa = &hpav2.HorizontalPodAutoscaler{
			ObjectMeta: makeObjectMeta(primaryHpaName, hpa.Labels, cd),
			Spec:       hpaSpec,
		}

		_, err = hr.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(cd.Namespace).Create(context.TODO(), primaryHpa, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating HorizontalPodAutoscaler v2 %s.%s failed: %w",
				primaryHpa.Name, primaryHpa.Namespace, err)
		}
		hr.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof(
			"HorizontalPodAutoscaler v2 %s.%s created", primaryHpa.GetName(), cd.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("HorizontalPodAutoscaler v2 %s.%s get query failed: %w",
			primaryHpa.Name, primaryHpa.Namespace, err)
	}

	// update HPA
	if !init && primaryHpa != nil {
		targetFields := hpaFields{
			metrics:     hpaSpec.Metrics,
			behavior:    hpaSpec.Behavior,
			annotations: hpa.Annotations,
			labels:      hpa.Labels,
			min:         hpaSpec.MinReplicas,
			max:         hpaSpec.MaxReplicas,
		}
		primaryFields := hpaFields{
			metrics:     primaryHpa.Spec.Metrics,
			behavior:    primaryHpa.Spec.Behavior,
			annotations: primaryHpa.Annotations,
			labels:      primaryHpa.Labels,
			min:         primaryHpa.Spec.MinReplicas,
			max:         primaryHpa.Spec.MaxReplicas,
		}
		if hasHPAChanged(targetFields, primaryFields) {
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				primaryHpa, err := hr.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				primaryHpaClone := primaryHpa.DeepCopy()
				primaryHpaClone.Spec.MaxReplicas = hpaSpec.MaxReplicas
				primaryHpaClone.Spec.MinReplicas = hpaSpec.MinReplicas
				primaryHpaClone.Spec.Metrics = hpaSpec.Metrics
				primaryHpaClone.Spec.Behavior = hpaSpec.Behavior

				hr.updateObjectMeta(primaryHpaClone.ObjectMeta, hpa.ObjectMeta)

				_, err = hr.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(cd.Namespace).Update(context.TODO(), primaryHpaClone, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				return fmt.Errorf("updating HorizontalPodAutoscaler v2 %s.%s failed: %w",
					primaryHpa.Name, primaryHpa.Namespace, err)
			}
			hr.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("HorizontalPodAutoscaler v2 %s.%s updated", primaryHpa.GetName(), cd.Namespace)
		}
	}
	return nil
}

func (hr *HPAReconciler) reconcilePrimaryHpaV2Beta2(cd *flaggerv1.Canary, hpa *hpav2beta2.HorizontalPodAutoscaler, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	hpaSpec := hpav2beta2.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: hpav2beta2.CrossVersionObjectReference{
			Name:       primaryName,
			Kind:       hpa.Spec.ScaleTargetRef.Kind,
			APIVersion: hpa.Spec.ScaleTargetRef.APIVersion,
		},
		MinReplicas: hpa.Spec.MinReplicas,
		MaxReplicas: hpa.Spec.MaxReplicas,
		Metrics:     hpa.Spec.Metrics,
		Behavior:    hpa.Spec.Behavior,
	}

	primaryHpaName := fmt.Sprintf("%s-primary", cd.Spec.AutoscalerRef.Name)
	primaryHpa, err := hr.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})

	// create HPA
	if errors.IsNotFound(err) {
		primaryHpa = &hpav2beta2.HorizontalPodAutoscaler{
			ObjectMeta: makeObjectMeta(primaryHpaName, hpa.Labels, cd),
			Spec:       hpaSpec,
		}

		_, err = hr.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Create(context.TODO(), primaryHpa, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating HorizontalPodAutoscaler v2beta2 %s.%s failed: %w",
				primaryHpa.Name, primaryHpa.Namespace, err)
		}
		hr.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof(
			"HorizontalPodAutoscaler v2beta2 %s.%s created", primaryHpa.GetName(), cd.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("HorizontalPodAutoscaler v2beta2 %s.%s get query failed: %w",
			primaryHpa.Name, primaryHpa.Namespace, err)
	}

	// update HPA
	if !init && primaryHpa != nil {
		targetFields := hpaFields{
			metrics:     hpaSpec.Metrics,
			behavior:    hpaSpec.Behavior,
			annotations: hpa.Annotations,
			labels:      hpa.Labels,
			min:         hpaSpec.MinReplicas,
			max:         hpaSpec.MaxReplicas,
		}
		primaryFields := hpaFields{
			metrics:     primaryHpa.Spec.Metrics,
			behavior:    primaryHpa.Spec.Behavior,
			annotations: primaryHpa.Annotations,
			labels:      primaryHpa.Labels,
			min:         primaryHpa.Spec.MinReplicas,
			max:         primaryHpa.Spec.MaxReplicas,
		}
		if hasHPAChanged(targetFields, primaryFields) {
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				primaryHpa, err := hr.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				primaryHpaClone := primaryHpa.DeepCopy()
				primaryHpaClone.Spec.MaxReplicas = hpaSpec.MaxReplicas
				primaryHpaClone.Spec.MinReplicas = hpaSpec.MinReplicas
				primaryHpaClone.Spec.Metrics = hpaSpec.Metrics
				primaryHpaClone.Spec.Behavior = hpaSpec.Behavior

				hr.updateObjectMeta(primaryHpaClone.ObjectMeta, hpa.ObjectMeta)

				_, err = hr.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Update(context.TODO(), primaryHpaClone, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				return fmt.Errorf("updating HorizontalPodAutoscaler v2beta2 %s.%s failed: %w",
					primaryHpa.Name, primaryHpa.Namespace, err)
			}
			hr.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("HorizontalPodAutoscaler v2beta2 %s.%s updated", primaryHpa.GetName(), cd.Namespace)
		}
	}
	return nil
}

func (hr *HPAReconciler) PauseTargetScaler(cd *flaggerv1.Canary) error {
	return nil
}

func (hr *HPAReconciler) ResumeTargetScaler(cd *flaggerv1.Canary) error {
	return nil
}

func (hr *HPAReconciler) updateObjectMeta(updateMeta, readMeta metav1.ObjectMeta) {
	// update hpa annotations
	filteredAnnotations := includeLabelsByPrefix(readMeta.Annotations, hr.includeLabelPrefix)
	updateMeta.Annotations = filteredAnnotations
	// update hpa labels
	filteredLabels := includeLabelsByPrefix(readMeta.Labels, hr.includeLabelPrefix)
	updateMeta.Labels = filteredLabels
}

type hpaFields struct {
	metrics     interface{}
	behavior    interface{}
	annotations map[string]string
	labels      map[string]string
	min         *int32
	max         int32
}

func hasHPAChanged(target, primary hpaFields) bool {
	diffMetrics := cmp.Diff(target.metrics, primary.metrics)
	diffBehavior := cmp.Diff(target.behavior, primary.behavior)
	diffLabels := cmp.Diff(target.labels, primary.labels)
	diffAnnotations := cmp.Diff(target.annotations, primary.annotations)
	if diffMetrics != "" || diffBehavior != "" || diffLabels != "" || diffAnnotations != "" ||
		int32Default(target.min) != int32Default(primary.min) || target.max != primary.max {
		return true
	}
	return false
}

func makeObjectMeta(name string, labels map[string]string, cd *flaggerv1.Canary) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: cd.Namespace,
		Labels:    filterMetadata(labels),
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(cd, schema.GroupVersionKind{
				Group:   flaggerv1.SchemeGroupVersion.Group,
				Version: flaggerv1.SchemeGroupVersion.Version,
				Kind:    flaggerv1.CanaryKind,
			}),
		},
	}
}
