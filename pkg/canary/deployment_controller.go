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

package canary

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav2 "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// DeploymentController is managing the operations for Kubernetes Deployment kind
type DeploymentController struct {
	kubeClient         kubernetes.Interface
	flaggerClient      clientset.Interface
	logger             *zap.SugaredLogger
	configTracker      Tracker
	labels             []string
	includeLabelPrefix []string
}

// Initialize creates the primary deployment if it does not exist.
func (c *DeploymentController) Initialize(cd *flaggerv1.Canary) (err error) {
	if err := c.createPrimaryDeployment(cd, c.includeLabelPrefix); err != nil {
		return fmt.Errorf("createPrimaryDeployment failed: %w", err)
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !cd.SkipAnalysis() {
			if err := c.IsPrimaryReady(cd); err != nil {
				return fmt.Errorf("%w", err)
			}
		}
	}

	return nil
}

// Promote copies the pod spec, secrets and config maps from canary to primary
func (c *DeploymentController) Promote(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
		}

		label, labelValue, err := c.getSelectorLabel(canary)
		primaryLabelValue := fmt.Sprintf("%s-primary", labelValue)
		if err != nil {
			return fmt.Errorf("getSelectorLabel failed: %w", err)
		}

		primary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s.%s get query error: %w", primaryName, cd.Namespace, err)
		}

		// promote secrets and config maps
		configRefs, err := c.configTracker.GetTargetConfigs(cd)
		if err != nil {
			return fmt.Errorf("GetTargetConfigs failed: %w", err)
		}
		if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs, c.includeLabelPrefix); err != nil {
			return fmt.Errorf("CreatePrimaryConfigs failed: %w", err)
		}

		primaryCopy := primary.DeepCopy()
		primaryCopy.Spec.ProgressDeadlineSeconds = canary.Spec.ProgressDeadlineSeconds
		primaryCopy.Spec.MinReadySeconds = canary.Spec.MinReadySeconds
		primaryCopy.Spec.RevisionHistoryLimit = canary.Spec.RevisionHistoryLimit
		primaryCopy.Spec.Strategy = canary.Spec.Strategy
		// update replica if hpa isn't set
		if cd.Spec.AutoscalerRef == nil {
			primaryCopy.Spec.Replicas = canary.Spec.Replicas
		}

		// update spec with primary secrets and config maps
		primaryCopy.Spec.Template.Spec = c.getPrimaryDeploymentTemplateSpec(canary, configRefs)

		// update pod annotations to ensure a rolling update
		podAnnotations, err := makeAnnotations(canary.Spec.Template.Annotations)
		if err != nil {
			return fmt.Errorf("makeAnnotations for podAnnotations failed: %w", err)
		}

		primaryCopy.Spec.Template.Annotations = podAnnotations
		primaryCopy.Spec.Template.Labels = makePrimaryLabels(canary.Spec.Template.Labels, primaryLabelValue, label)

		// update deploy annotations
		primaryCopy.ObjectMeta.Annotations = make(map[string]string)
		filteredAnnotations := includeLabelsByPrefix(canary.ObjectMeta.Annotations, c.includeLabelPrefix)
		for k, v := range filteredAnnotations {
			primaryCopy.ObjectMeta.Annotations[k] = v
		}
		// update deploy labels
		filteredLabels := includeLabelsByPrefix(canary.ObjectMeta.Labels, c.includeLabelPrefix)
		primaryCopy.ObjectMeta.Labels = makePrimaryLabels(filteredLabels, primaryLabelValue, label)

		// apply update
		_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Update(context.TODO(), primaryCopy, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("updating deployment %s.%s template spec failed: %w",
			primaryName, cd.Namespace, err)
	}

	return nil
}

// HasTargetChanged returns true if the canary deployment pod spec has changed
func (c *DeploymentController) HasTargetChanged(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	return hasSpecChanged(cd, canary.Spec.Template)
}

// ScaleToZero Scale sets the canary deployment replicas
func (c *DeploymentController) ScaleToZero(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = int32p(0)

	_, err = c.kubeClient.AppsV1().Deployments(dep.Namespace).Update(context.TODO(), depCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s update query error: %w", targetName, cd.Namespace, err)
	}
	return nil
}

func (c *DeploymentController) ScaleFromZero(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	replicas := int32p(1)
	if dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
		replicas = dep.Spec.Replicas
	} else if cd.Spec.AutoscalerRef == nil {
		// If HPA isn't set and replicas are not specified, it uses the primary replicas when scaling up the canary
		primaryName := fmt.Sprintf("%s-primary", targetName)
		primary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s.%s get query error: %w", primaryName, cd.Namespace, err)
		}

		if primary.Spec.Replicas != nil && *primary.Spec.Replicas > 0 {
			replicas = primary.Spec.Replicas
		}
	} else if cd.Spec.AutoscalerRef != nil {
		if cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
			hpa, err := c.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
			if err == nil {
				if hpa.Spec.MinReplicas != nil && *hpa.Spec.MinReplicas > 1 {
					replicas = hpa.Spec.MinReplicas
				}
			} else {
				// fallback to v2beta2
				hpa, err := c.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
				if err == nil {
					if hpa.Spec.MinReplicas != nil && *hpa.Spec.MinReplicas > 1 {
						replicas = hpa.Spec.MinReplicas
					}
				}
			}
		} else if cd.Spec.AutoscalerRef.Kind == "ScaledObject" {
			so, err := c.flaggerClient.KedaV1alpha1().ScaledObjects(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
			if err == nil {
				if so.Spec.MinReplicaCount != nil && *so.Spec.MinReplicaCount > 1 {
					replicas = so.Spec.MinReplicaCount
				}
			}
		}
	}
	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = replicas

	_, err = c.kubeClient.AppsV1().Deployments(dep.Namespace).Update(context.TODO(), depCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("scaling up %s.%s to %v failed: %v", depCopy.GetName(), depCopy.Namespace, replicas, err)
	}
	return nil
}

// GetMetadata returns the pod label selector and svc ports
func (c *DeploymentController) GetMetadata(cd *flaggerv1.Canary) (string, string, map[string]int32, error) {
	targetName := cd.Spec.TargetRef.Name

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return "", "", nil, fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	label, labelValue, err := c.getSelectorLabel(canaryDep)
	if err != nil {
		return "", "", nil, fmt.Errorf("getSelectorLabel failed: %w", err)
	}

	var ports map[string]int32
	if cd.Spec.Service.PortDiscovery {
		ports = getPorts(cd, canaryDep.Spec.Template.Spec.Containers)
	}

	return label, labelValue, ports, nil
}
func (c *DeploymentController) createPrimaryDeployment(cd *flaggerv1.Canary, includeLabelPrefix []string) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	// Create the labels map but filter unwanted labels
	labels := includeLabelsByPrefix(canaryDep.Labels, includeLabelPrefix)

	label, labelValue, err := c.getSelectorLabel(canaryDep)
	primaryLabelValue := fmt.Sprintf("%s-primary", labelValue)
	if err != nil {
		return fmt.Errorf("getSelectorLabel failed: %w", err)
	}

	primaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// create primary secrets and config maps
		configRefs, err := c.configTracker.GetTargetConfigs(cd)
		if err != nil {
			return fmt.Errorf("GetTargetConfigs failed: %w", err)
		}
		if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs, c.includeLabelPrefix); err != nil {
			return fmt.Errorf("CreatePrimaryConfigs failed: %w", err)
		}
		annotations, err := makeAnnotations(canaryDep.Spec.Template.Annotations)
		if err != nil {
			return fmt.Errorf("makeAnnotations failed: %w", err)
		}

		replicas := int32(1)
		if canaryDep.Spec.Replicas != nil && *canaryDep.Spec.Replicas > 0 {
			replicas = *canaryDep.Spec.Replicas
		}

		// create primary deployment
		primaryDep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        primaryName,
				Namespace:   cd.Namespace,
				Labels:      makePrimaryLabels(labels, primaryLabelValue, label),
				Annotations: filterMetadata(canaryDep.Annotations),
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: appsv1.DeploymentSpec{
				ProgressDeadlineSeconds: canaryDep.Spec.ProgressDeadlineSeconds,
				MinReadySeconds:         canaryDep.Spec.MinReadySeconds,
				RevisionHistoryLimit:    canaryDep.Spec.RevisionHistoryLimit,
				Replicas:                int32p(replicas),
				Strategy:                canaryDep.Spec.Strategy,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						label: primaryLabelValue,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      makePrimaryLabels(canaryDep.Spec.Template.Labels, primaryLabelValue, label),
						Annotations: annotations,
					},
					// update spec with the primary secrets and config maps
					Spec: c.getPrimaryDeploymentTemplateSpec(canaryDep, configRefs),
				},
			},
		}

		_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Create(context.TODO(), primaryDep, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating deployment %s.%s failed: %w", primaryDep.Name, cd.Namespace, err)
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
			Infof("Deployment %s.%s created", primaryDep.GetName(), cd.Namespace)
	}

	return nil
}

func (c *DeploymentController) reconcilePrimaryHpa(cd *flaggerv1.Canary, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	hpa, err := c.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("HorizontalPodAutoscaler %s.%s get query error: %w",
			cd.Spec.AutoscalerRef.Name, cd.Namespace, err)
	}

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
	primaryHpa, err := c.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})

	// create HPA
	if errors.IsNotFound(err) {
		primaryHpa = &hpav2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      primaryHpaName,
				Namespace: cd.Namespace,
				Labels:    filterMetadata(hpa.Labels),
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: hpaSpec,
		}

		_, err = c.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Create(context.TODO(), primaryHpa, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating HorizontalPodAutoscaler %s.%s failed: %w",
				primaryHpa.Name, primaryHpa.Namespace, err)
		}
		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof(
			"HorizontalPodAutoscaler %s.%s created", primaryHpa.GetName(), cd.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("HorizontalPodAutoscaler %s.%s get query failed: %w",
			primaryHpa.Name, primaryHpa.Namespace, err)
	}

	// update HPA
	if !init && primaryHpa != nil {
		diffMetrics := cmp.Diff(hpaSpec.Metrics, primaryHpa.Spec.Metrics)
		diffBehavior := cmp.Diff(hpaSpec.Behavior, primaryHpa.Spec.Behavior)
		diffLabels := cmp.Diff(hpa.ObjectMeta.Labels, primaryHpa.ObjectMeta.Labels)
		diffAnnotations := cmp.Diff(hpa.ObjectMeta.Annotations, primaryHpa.ObjectMeta.Annotations)
		if diffMetrics != "" || diffBehavior != "" || diffLabels != "" || diffAnnotations != "" || int32Default(hpaSpec.MinReplicas) != int32Default(primaryHpa.Spec.MinReplicas) || hpaSpec.MaxReplicas != primaryHpa.Spec.MaxReplicas {
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				primaryHpa, err := c.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				hpaClone := primaryHpa.DeepCopy()
				hpaClone.Spec.MaxReplicas = hpaSpec.MaxReplicas
				hpaClone.Spec.MinReplicas = hpaSpec.MinReplicas
				hpaClone.Spec.Metrics = hpaSpec.Metrics
				hpaClone.Spec.Behavior = hpaSpec.Behavior

				// update hpa annotations
				hpaClone.ObjectMeta.Annotations = make(map[string]string)
				filteredAnnotations := includeLabelsByPrefix(hpa.ObjectMeta.Annotations, c.includeLabelPrefix)
				for k, v := range filteredAnnotations {
					hpaClone.ObjectMeta.Annotations[k] = v
				}
				// update hpa labels
				hpaClone.ObjectMeta.Labels = make(map[string]string)
				filteredLabels := includeLabelsByPrefix(hpa.ObjectMeta.Labels, c.includeLabelPrefix)
				for k, v := range filteredLabels {
					hpaClone.ObjectMeta.Labels[k] = v
				}

				_, err = c.kubeClient.AutoscalingV2beta2().HorizontalPodAutoscalers(cd.Namespace).Update(context.TODO(), hpaClone, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				return fmt.Errorf("updating HorizontalPodAutoscaler %s.%s failed: %w",
					primaryHpa.Name, primaryHpa.Namespace, err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("HorizontalPodAutoscaler %s.%s updated", primaryHpa.GetName(), cd.Namespace)
		}
	}
	return nil
}

// getSelectorLabel returns the selector match label
func (c *DeploymentController) getSelectorLabel(deployment *appsv1.Deployment) (string, string, error) {
	for _, l := range c.labels {
		if _, ok := deployment.Spec.Selector.MatchLabels[l]; ok {
			return l, deployment.Spec.Selector.MatchLabels[l], nil
		}
	}

	return "", "", fmt.Errorf(
		"deployment %s.%s spec.selector.matchLabels must contain one of %v",
		deployment.Name, deployment.Namespace, c.labels,
	)
}

func (c *DeploymentController) HaveDependenciesChanged(cd *flaggerv1.Canary) (bool, error) {
	return c.configTracker.HasConfigChanged(cd)
}

// Finalize will set the replica count from the primary to the reference instance.  This method is used
// during a delete to attempt to revert the deployment back to the original state.  Error is returned if unable
// update the reference deployment replicas to the primary replicas
func (c *DeploymentController) Finalize(cd *flaggerv1.Canary) error {

	// get ref deployment
	refDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deplyoment %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	// get primary if possible, if not scale from zero
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			if err := c.ScaleFromZero(cd); err != nil {
				return fmt.Errorf("ScaleFromZero failed: %w", err)
			}
			return nil
		}
		return fmt.Errorf("deplyoment %s.%s get query error: %w", primaryName, cd.Namespace, err)
	}

	// if both ref and primary present update the replicas of the ref to match the primary
	if refDep.Spec.Replicas != primaryDep.Spec.Replicas {
		// set the replicas value on the original reference deployment
		if err := c.scale(cd, int32Default(primaryDep.Spec.Replicas)); err != nil {
			return fmt.Errorf("scale failed: %w", err)
		}
	}
	return nil
}

// Scale sets the canary deployment replicas
func (c *DeploymentController) scale(cd *flaggerv1.Canary, replicas int32) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s query error: %w", targetName, cd.Namespace, err)
	}

	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = int32p(replicas)
	_, err = c.kubeClient.AppsV1().Deployments(dep.Namespace).Update(context.TODO(), depCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v failed: %w", depCopy.GetName(), depCopy.Namespace, replicas, err)
	}
	return nil
}

func (c *DeploymentController) getPrimaryDeploymentTemplateSpec(canaryDep *appsv1.Deployment, refs map[string]ConfigRef) corev1.PodSpec {
	spec := c.configTracker.ApplyPrimaryConfigs(canaryDep.Spec.Template.Spec, refs)

	// update TopologySpreadConstraints
	for _, topologySpreadConstraint := range spec.TopologySpreadConstraints {
		c.appendPrimarySuffixToValuesIfNeeded(topologySpreadConstraint.LabelSelector, canaryDep)
	}

	// update affinity
	if affinity := spec.Affinity; affinity != nil {
		if podAntiAffinity := affinity.PodAntiAffinity; podAntiAffinity != nil {
			for _, preferredAntiAffinity := range podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
				c.appendPrimarySuffixToValuesIfNeeded(preferredAntiAffinity.PodAffinityTerm.LabelSelector, canaryDep)
			}

			for _, requiredAntiAffinity := range podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
				c.appendPrimarySuffixToValuesIfNeeded(requiredAntiAffinity.LabelSelector, canaryDep)
			}
		}
	}

	return spec
}

func (c *DeploymentController) appendPrimarySuffixToValuesIfNeeded(labelSelector *metav1.LabelSelector, canaryDep *appsv1.Deployment) {
	if labelSelector != nil {
		for _, matchExpression := range labelSelector.MatchExpressions {
			if contains(c.labels, matchExpression.Key) {
				for i := range matchExpression.Values {
					if matchExpression.Values[i] == canaryDep.Name {
						matchExpression.Values[i] += "-primary"
						break
					}
				}
			}
		}

		for key, value := range labelSelector.MatchLabels {
			if contains(c.labels, key) {
				if value == canaryDep.Name {
					labelSelector.MatchLabels[key] = value + "-primary"
				}
			}
		}
	}
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
