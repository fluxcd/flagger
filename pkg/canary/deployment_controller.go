package canary

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// DeploymentController is managing the operations for Kubernetes Deployment kind
type DeploymentController struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker Tracker
	labels        []string
}

// Initialize creates the primary deployment, hpa,
// scales to zero the canary deployment and returns the pod selector label and container ports
func (c *DeploymentController) Initialize(cd *flaggerv1.Canary) (err error) {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	if err := c.createPrimaryDeployment(cd); err != nil {
		return fmt.Errorf("createPrimaryDeployment failed: %w", err)
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !cd.SkipAnalysis() {
			if err := c.IsPrimaryReady(cd); err != nil {
				return fmt.Errorf("%w", err)
			}
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
			Infof("Scaling down Deployment %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		if err := c.ScaleToZero(cd); err != nil {
			return fmt.Errorf("scaling down canary deployment %s.%s failed: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
		}
	}

	if cd.Spec.AutoscalerRef != nil {
		if cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
			if err := c.reconcilePrimaryHpa(cd, true); err != nil {
				return fmt.Errorf(
					"initial reconcilePrimaryHpa for %s.%s failed: %w", primaryName, cd.Namespace, err)
			}
		} else {
			return fmt.Errorf("cd.Spec.AutoscalerRef.Kind is invalid: %s", cd.Spec.AutoscalerRef.Kind)
		}
	}
	return nil
}

// Promote copies the pod spec, secrets and config maps from canary to primary
func (c *DeploymentController) Promote(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	label, err := c.getSelectorLabel(canary)
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
	if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
		return fmt.Errorf("CreatePrimaryConfigs failed: %w", err)
	}

	primaryCopy := primary.DeepCopy()
	primaryCopy.Spec.ProgressDeadlineSeconds = canary.Spec.ProgressDeadlineSeconds
	primaryCopy.Spec.MinReadySeconds = canary.Spec.MinReadySeconds
	primaryCopy.Spec.RevisionHistoryLimit = canary.Spec.RevisionHistoryLimit
	primaryCopy.Spec.Strategy = canary.Spec.Strategy

	// update spec with primary secrets and config maps
	primaryCopy.Spec.Template.Spec = c.configTracker.ApplyPrimaryConfigs(canary.Spec.Template.Spec, configRefs)

	// update pod annotations to ensure a rolling update
	annotations, err := makeAnnotations(canary.Spec.Template.Annotations)
	if err != nil {
		return fmt.Errorf("makeAnnotations failed: %w", err)
	}

	primaryCopy.Spec.Template.Annotations = annotations
	primaryCopy.Spec.Template.Labels = makePrimaryLabels(canary.Spec.Template.Labels, primaryName, label)

	// apply update
	_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Update(context.TODO(), primaryCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating deployment %s.%s template spec failed: %w",
			primaryCopy.GetName(), primaryCopy.Namespace, err)
	}

	// update HPA
	if cd.Spec.AutoscalerRef != nil {
		if cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
			if err := c.reconcilePrimaryHpa(cd, false); err != nil {
				return fmt.Errorf(
					"reconcilePrimaryHpa for %s.%s failed: %w", primaryName, cd.Namespace, err)
			}
		} else {
			return fmt.Errorf("cd.Spec.AutoscalerRef.Kind is invalid: %s", cd.Spec.AutoscalerRef.Kind)
		}
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

// Scale sets the canary deployment replicas
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
func (c *DeploymentController) GetMetadata(cd *flaggerv1.Canary) (string, map[string]int32, error) {
	targetName := cd.Spec.TargetRef.Name

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	label, err := c.getSelectorLabel(canaryDep)
	if err != nil {
		return "", nil, fmt.Errorf("getSelectorLabel failed: %w", err)
	}

	var ports map[string]int32
	if cd.Spec.Service.PortDiscovery {
		ports = getPorts(cd, canaryDep.Spec.Template.Spec.Containers)
	}

	return label, ports, nil
}
func (c *DeploymentController) createPrimaryDeployment(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deplyoment %s.%s get query error: %w", targetName, cd.Namespace, err)
	}

	label, err := c.getSelectorLabel(canaryDep)
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
		if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
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
				Name:      primaryName,
				Namespace: cd.Namespace,
				Labels: map[string]string{
					label: primaryName,
				},
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
						label: primaryName,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      makePrimaryLabels(canaryDep.Spec.Template.Labels, primaryName, label),
						Annotations: annotations,
					},
					// update spec with the primary secrets and config maps
					Spec: c.configTracker.ApplyPrimaryConfigs(canaryDep.Spec.Template.Spec, configRefs),
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
	hpa, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("HorizontalPodAutoscaler %s.%s get query error: %w",
			cd.Spec.AutoscalerRef.Name, cd.Namespace, err)
	}

	hpaSpec := hpav1.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: hpav1.CrossVersionObjectReference{
			Name:       primaryName,
			Kind:       hpa.Spec.ScaleTargetRef.Kind,
			APIVersion: hpa.Spec.ScaleTargetRef.APIVersion,
		},
		MinReplicas: hpa.Spec.MinReplicas,
		MaxReplicas: hpa.Spec.MaxReplicas,
		Metrics:     hpa.Spec.Metrics,
	}

	primaryHpaName := fmt.Sprintf("%s-primary", cd.Spec.AutoscalerRef.Name)
	primaryHpa, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(context.TODO(), primaryHpaName, metav1.GetOptions{})

	// create HPA
	if errors.IsNotFound(err) {
		primaryHpa = &hpav1.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      primaryHpaName,
				Namespace: cd.Namespace,
				Labels:    hpa.Labels,
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

		_, err = c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Create(context.TODO(), primaryHpa, metav1.CreateOptions{})
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
		diff := cmp.Diff(hpaSpec.Metrics, primaryHpa.Spec.Metrics)
		if diff != "" || int32Default(hpaSpec.MinReplicas) != int32Default(primaryHpa.Spec.MinReplicas) || hpaSpec.MaxReplicas != primaryHpa.Spec.MaxReplicas {
			fmt.Println(diff, hpaSpec.MinReplicas, primaryHpa.Spec.MinReplicas, hpaSpec.MaxReplicas, primaryHpa.Spec.MaxReplicas)
			hpaClone := primaryHpa.DeepCopy()
			hpaClone.Spec.MaxReplicas = hpaSpec.MaxReplicas
			hpaClone.Spec.MinReplicas = hpaSpec.MinReplicas
			hpaClone.Spec.Metrics = hpaSpec.Metrics

			_, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Update(context.TODO(), hpaClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("updating HorizontalPodAutoscaler %s.%s failed: %w",
					hpaClone.Name, hpaClone.Namespace, err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("HorizontalPodAutoscaler %s.%s updated", primaryHpa.GetName(), cd.Namespace)
		}
	}
	return nil
}

// getSelectorLabel returns the selector match label
func (c *DeploymentController) getSelectorLabel(deployment *appsv1.Deployment) (string, error) {
	for _, l := range c.labels {
		if _, ok := deployment.Spec.Selector.MatchLabels[l]; ok {
			return l, nil
		}
	}

	return "", fmt.Errorf(
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
