package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// CanaryDeployer is managing the operations for Kubernetes deployment kind
type CanaryDeployer struct {
	kubeClient    kubernetes.Interface
	istioClient   istioclientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Promote copies the pod spec from canary to primary
func (c *CanaryDeployer) Promote(cd *flaggerv1.Canary) error {
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", primaryName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", primaryName, cd.Namespace, err)
	}

	primary.Spec.ProgressDeadlineSeconds = canary.Spec.ProgressDeadlineSeconds
	primary.Spec.MinReadySeconds = canary.Spec.MinReadySeconds
	primary.Spec.RevisionHistoryLimit = canary.Spec.RevisionHistoryLimit
	primary.Spec.Strategy = canary.Spec.Strategy
	primary.Spec.Template.Spec = canary.Spec.Template.Spec
	_, err = c.kubeClient.AppsV1().Deployments(primary.Namespace).Update(primary)
	if err != nil {
		return fmt.Errorf("updating template spec %s.%s failed: %v", primary.GetName(), primary.Namespace, err)

	}

	return nil
}

// IsPrimaryReady checks the primary deployment status and returns an error if
// the deployment is in the middle of a rolling update or if the pods are unhealthy
// it will return a non retriable error if the rolling update is stuck
func (c *CanaryDeployer) IsPrimaryReady(cd *flaggerv1.Canary) (bool, error) {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	primary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return true, fmt.Errorf("deployment %s.%s not found", primaryName, cd.Namespace)
		}
		return true, fmt.Errorf("deployment %s.%s query error %v", primaryName, cd.Namespace, err)
	}

	retriable, err := c.isDeploymentReady(primary, cd.GetProgressDeadlineSeconds())
	if err != nil {
		if retriable {
			return retriable, fmt.Errorf("Halt %s.%s advancement %s", cd.Name, cd.Namespace, err.Error())
		} else {
			return retriable, err
		}
	}

	if primary.Spec.Replicas == int32p(0) {
		return true, fmt.Errorf("halt %s.%s advancement primary deployment is scaled to zero",
			cd.Name, cd.Namespace)
	}
	return true, nil
}

// IsCanaryReady checks the primary deployment status and returns an error if
// the deployment is in the middle of a rolling update or if the pods are unhealthy
// it will return a non retriable error if the rolling update is stuck
func (c *CanaryDeployer) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return true, fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return true, fmt.Errorf("deployment %s.%s query error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	retriable, err := c.isDeploymentReady(canary, cd.GetProgressDeadlineSeconds())
	if err != nil {
		if retriable {
			return retriable, fmt.Errorf("Halt %s.%s advancement %s", cd.Name, cd.Namespace, err.Error())
		} else {
			return retriable, fmt.Errorf("deployment does not have minimum availability for more than %vs",
				cd.GetProgressDeadlineSeconds())
		}
	}

	return true, nil
}

// IsNewSpec returns true if the canary deployment pod spec has changed
func (c *CanaryDeployer) IsNewSpec(cd *flaggerv1.Canary) (bool, error) {
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return false, fmt.Errorf("deployment %s.%s query error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	if cd.Status.CanaryRevision == "" {
		return true, nil
	}

	newSpec := &canary.Spec.Template.Spec
	oldSpecJson, err := base64.StdEncoding.DecodeString(cd.Status.CanaryRevision)
	if err != nil {
		return false, err
	}
	oldSpec := &corev1.PodSpec{}
	err = json.Unmarshal(oldSpecJson, oldSpec)
	if err != nil {
		return false, fmt.Errorf("%s.%s unmarshal error %v", cd.Name, cd.Namespace, err)
	}

	if diff := cmp.Diff(*newSpec, *oldSpec, cmpopts.IgnoreUnexported(resource.Quantity{})); diff != "" {
		//fmt.Println(diff)
		return true, nil
	}

	return false, nil
}

// SetFailedChecks updates the canary failed checks counter
func (c *CanaryDeployer) SetFailedChecks(cd *flaggerv1.Canary, val int) error {
	cd.Status.FailedChecks = val
	cd.Status.LastTransitionTime = metav1.Now()
	cd, err := c.flaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).Update(cd)
	if err != nil {
		return fmt.Errorf("deployment %s.%s update error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return nil
}

// SetState updates the canary status state
func (c *CanaryDeployer) SetState(cd *flaggerv1.Canary, state flaggerv1.CanaryState) error {
	cd.Status.State = state
	cd.Status.LastTransitionTime = metav1.Now()
	cd, err := c.flaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).Update(cd)
	if err != nil {
		return fmt.Errorf("deployment %s.%s update error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return nil
}

// SyncStatus encodes the canary pod spec and updates the canary status
func (c *CanaryDeployer) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	specJson, err := json.Marshal(canary.Spec.Template.Spec)
	if err != nil {
		return fmt.Errorf("deployment %s.%s marshal error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	specEnc := base64.StdEncoding.EncodeToString(specJson)
	cd.Status.State = status.State
	cd.Status.FailedChecks = status.FailedChecks
	cd.Status.CanaryRevision = specEnc
	cd.Status.LastTransitionTime = metav1.Now()
	_, err = c.flaggerClient.FlaggerV1alpha3().Canaries(cd.Namespace).Update(cd)
	if err != nil {
		return fmt.Errorf("deployment %s.%s update error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return nil
}

// Scale sets the canary deployment replicas
func (c *CanaryDeployer) Scale(cd *flaggerv1.Canary, replicas int32) error {
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", cd.Spec.TargetRef.Name, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	canary.Spec.Replicas = int32p(replicas)
	canary, err = c.kubeClient.AppsV1().Deployments(canary.Namespace).Update(canary)
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v failed: %v", canary.GetName(), canary.Namespace, replicas, err)
	}
	return nil
}

// Sync creates the primary deployment and hpa
// and scales to zero the canary deployment
func (c *CanaryDeployer) Sync(cd *flaggerv1.Canary) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	if err := c.createPrimaryDeployment(cd); err != nil {
		return fmt.Errorf("creating deployment %s.%s failed: %v", primaryName, cd.Namespace, err)
	}

	if cd.Status.State == "" {
		c.logger.Infof("Scaling down %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		if err := c.Scale(cd, 0); err != nil {
			return err
		}
	}

	if cd.Spec.AutoscalerRef != nil && cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
		if err := c.createPrimaryHpa(cd); err != nil {
			return fmt.Errorf("creating hpa %s.%s failed: %v", primaryName, cd.Namespace, err)
		}
	}
	return nil
}

func (c *CanaryDeployer) createPrimaryDeployment(cd *flaggerv1.Canary) error {
	canaryName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(canaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found, retrying", canaryName, cd.Namespace)
		}
		return err
	}

	primaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		primaryDep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        primaryName,
				Annotations: canaryDep.Annotations,
				Namespace:   cd.Namespace,
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
				Replicas:                canaryDep.Spec.Replicas,
				Strategy:                canaryDep.Spec.Strategy,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": primaryName,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app": primaryName},
						Annotations: canaryDep.Spec.Template.Annotations,
					},
					Spec: canaryDep.Spec.Template.Spec,
				},
			},
		}

		_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Create(primaryDep)
		if err != nil {
			return err
		}

		c.logger.Infof("Deployment %s.%s created", primaryDep.GetName(), cd.Namespace)
	}

	return nil
}

func (c *CanaryDeployer) createPrimaryHpa(cd *flaggerv1.Canary) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	hpa, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("HorizontalPodAutoscaler %s.%s not found, retrying",
				cd.Spec.AutoscalerRef.Name, cd.Namespace)
		}
		return err
	}
	primaryHpaName := fmt.Sprintf("%s-primary", cd.Spec.AutoscalerRef.Name)
	primaryHpa, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(primaryHpaName, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		primaryHpa = &hpav1.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      primaryHpaName,
				Namespace: cd.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(cd, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: hpav1.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: hpav1.CrossVersionObjectReference{
					Name:       primaryName,
					Kind:       hpa.Spec.ScaleTargetRef.Kind,
					APIVersion: hpa.Spec.ScaleTargetRef.APIVersion,
				},
				MinReplicas: hpa.Spec.MinReplicas,
				MaxReplicas: hpa.Spec.MaxReplicas,
				Metrics:     hpa.Spec.Metrics,
			},
		}

		_, err = c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Create(primaryHpa)
		if err != nil {
			return err
		}
		c.logger.Infof("HorizontalPodAutoscaler %s.%s created", primaryHpa.GetName(), cd.Namespace)
	}

	return nil
}

// isDeploymentReady determines if a deployment is ready by checking the status conditions
// if a deployment has exceeded the progress deadline it returns a non retriable error
func (c *CanaryDeployer) isDeploymentReady(deployment *appsv1.Deployment, deadline int) (bool, error) {
	retriable := true
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		progress := c.getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
		if progress != nil {
			// Determine if the deployment is stuck by checking if there is a minimum replicas unavailable condition
			// and if the last update time exceeds the deadline
			available := c.getDeploymentCondition(deployment.Status, appsv1.DeploymentAvailable)
			if available != nil && available.Status == "False" && available.Reason == "MinimumReplicasUnavailable" {
				from := available.LastUpdateTime
				delta := time.Duration(deadline) * time.Second
				retriable = !from.Add(delta).Before(time.Now())
			}
		}

		if progress != nil && progress.Reason == "ProgressDeadlineExceeded" {
			return false, fmt.Errorf("deployment %q exceeded its progress deadline", deployment.GetName())
		} else if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d out of %d new replicas have been updated",
				deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas)
		} else if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d old replicas are pending termination",
				deployment.Status.Replicas-deployment.Status.UpdatedReplicas)
		} else if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d of %d updated replicas are available",
				deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas)
		}

	} else {
		return true, fmt.Errorf("waiting for rollout to finish: observed deployment generation less then desired generation")
	}

	return true, nil
}

func (c *CanaryDeployer) getDeploymentCondition(
	status appsv1.DeploymentStatus,
	conditionType appsv1.DeploymentConditionType,
) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}
