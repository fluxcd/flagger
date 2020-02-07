package canary

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// DeploymentController is managing the operations for Kubernetes Deployment kind
type DeploymentController struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker ConfigTracker
	labels        []string
}

// Initialize creates the primary deployment, hpa,
// scales to zero the canary deployment and returns the pod selector label and container ports
func (c *DeploymentController) Initialize(cd *flaggerv1.Canary, skipLivenessChecks bool) (err error) {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	err = c.createPrimaryDeployment(cd)
	if err != nil {
		return fmt.Errorf("creating deployment %s.%s failed: %v", primaryName, cd.Namespace, err)
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !skipLivenessChecks && !cd.Spec.SkipAnalysis {
			_, readyErr := c.IsPrimaryReady(cd)
			if readyErr != nil {
				return readyErr
			}
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Scaling down %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		if err := c.Scale(cd, 0); err != nil {
			return err
		}
	}

	if cd.Spec.AutoscalerRef != nil && cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
		if err := c.reconcilePrimaryHpa(cd, true); err != nil {
			return fmt.Errorf("creating HorizontalPodAutoscaler %s.%s failed: %v", primaryName, cd.Namespace, err)
		}
	}
	return nil
}

// Promote copies the pod spec, secrets and config maps from canary to primary
func (c *DeploymentController) Promote(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	label, err := c.getSelectorLabel(canary)
	if err != nil {
		return fmt.Errorf("invalid label selector! Deployment %s.%s spec.selector.matchLabels must contain selector 'app: %s'",
			targetName, cd.Namespace, targetName)
	}

	primary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", primaryName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", primaryName, cd.Namespace, err)
	}

	// promote secrets and config maps
	configRefs, err := c.configTracker.GetTargetConfigs(cd)
	if err != nil {
		return err
	}
	if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
		return err
	}

	primaryCopy := primary.DeepCopy()
	primaryCopy.Spec.ProgressDeadlineSeconds = canary.Spec.ProgressDeadlineSeconds
	primaryCopy.Spec.MinReadySeconds = canary.Spec.MinReadySeconds
	primaryCopy.Spec.RevisionHistoryLimit = canary.Spec.RevisionHistoryLimit
	primaryCopy.Spec.Strategy = canary.Spec.Strategy

	// update spec with primary secrets and config maps
	primaryCopy.Spec.Template.Spec = c.configTracker.ApplyPrimaryConfigs(canary.Spec.Template.Spec, configRefs)

	// update pod annotations to ensure a rolling update
	annotations, err := c.makeAnnotations(canary.Spec.Template.Annotations)
	if err != nil {
		return err
	}
	primaryCopy.Spec.Template.Annotations = annotations

	primaryCopy.Spec.Template.Labels = makePrimaryLabels(canary.Spec.Template.Labels, primaryName, label)

	// apply update
	_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Update(primaryCopy)
	if err != nil {
		return fmt.Errorf("updating deployment %s.%s template spec failed: %v",
			primaryCopy.GetName(), primaryCopy.Namespace, err)
	}

	// update HPA
	if cd.Spec.AutoscalerRef != nil && cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
		if err := c.reconcilePrimaryHpa(cd, false); err != nil {
			return fmt.Errorf("updating HorizontalPodAutoscaler %s.%s failed: %v", primaryName, cd.Namespace, err)
		}
	}

	return nil
}

// HasTargetChanged returns true if the canary deployment pod spec has changed
func (c *DeploymentController) HasTargetChanged(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return false, fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	return hasSpecChanged(cd, canary.Spec.Template)
}

// Scale sets the canary deployment replicas
func (c *DeploymentController) Scale(cd *flaggerv1.Canary, replicas int32) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = int32p(replicas)

	_, err = c.kubeClient.AppsV1().Deployments(dep.Namespace).Update(depCopy)
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v failed: %v", depCopy.GetName(), depCopy.Namespace, replicas, err)
	}
	return nil
}

func (c *DeploymentController) ScaleFromZero(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	replicas := int32p(1)
	if dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
		replicas = dep.Spec.Replicas
	}
	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = replicas

	_, err = c.kubeClient.AppsV1().Deployments(dep.Namespace).Update(depCopy)
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v failed: %v", depCopy.GetName(), depCopy.Namespace, replicas, err)
	}
	return nil
}

// GetMetadata returns the pod label selector and svc ports
func (c *DeploymentController) GetMetadata(cd *flaggerv1.Canary) (string, map[string]int32, error) {
	targetName := cd.Spec.TargetRef.Name

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil, fmt.Errorf("deployment %s.%s not found, retrying", targetName, cd.Namespace)
		}
		return "", nil, err
	}

	label, err := c.getSelectorLabel(canaryDep)
	if err != nil {
		return "", nil, fmt.Errorf("invalid label selector! Deployment %s.%s spec.selector.matchLabels must contain selector 'app: %s'",
			targetName, cd.Namespace, targetName)
	}

	var ports map[string]int32
	if cd.Spec.Service.PortDiscovery {
		p, err := c.getPorts(cd, canaryDep)
		if err != nil {
			return "", nil, fmt.Errorf("port discovery failed with error: %v", err)
		}
		ports = p
	}

	return label, ports, nil
}
func (c *DeploymentController) createPrimaryDeployment(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found, retrying", targetName, cd.Namespace)
		}
		return err
	}

	label, err := c.getSelectorLabel(canaryDep)
	if err != nil {
		return fmt.Errorf("invalid label selector! Deployment %s.%s spec.selector.matchLabels must contain selector 'app: %s'",
			targetName, cd.Namespace, targetName)
	}

	primaryDep, err := c.kubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// create primary secrets and config maps
		configRefs, err := c.configTracker.GetTargetConfigs(cd)
		if err != nil {
			return err
		}
		if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
			return err
		}
		annotations, err := c.makeAnnotations(canaryDep.Spec.Template.Annotations)
		if err != nil {
			return err
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

		_, err = c.kubeClient.AppsV1().Deployments(cd.Namespace).Create(primaryDep)
		if err != nil {
			return err
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Deployment %s.%s created", primaryDep.GetName(), cd.Namespace)
	}

	return nil
}

func (c *DeploymentController) reconcilePrimaryHpa(cd *flaggerv1.Canary, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	hpa, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("HorizontalPodAutoscaler %s.%s not found, retrying",
				cd.Spec.AutoscalerRef.Name, cd.Namespace)
		}
		return err
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
	primaryHpa, err := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(primaryHpaName, metav1.GetOptions{})

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

		_, err = c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Create(primaryHpa)
		if err != nil {
			return err
		}
		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("HorizontalPodAutoscaler %s.%s created", primaryHpa.GetName(), cd.Namespace)
		return nil
	}

	if err != nil {
		return err
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

			_, upErr := c.kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Update(hpaClone)
			if upErr != nil {
				return upErr
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("HorizontalPodAutoscaler %s.%s updated", primaryHpa.GetName(), cd.Namespace)
		}
	}

	return nil
}

// makeAnnotations appends an unique ID to annotations map
func (c *DeploymentController) makeAnnotations(annotations map[string]string) (map[string]string, error) {
	idKey := "flagger-id"
	res := make(map[string]string)
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return res, err
	}
	uuid[8] = uuid[8]&^0xc0 | 0x80
	uuid[6] = uuid[6]&^0xf0 | 0x40
	id := fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])

	for k, v := range annotations {
		if k != idKey {
			res[k] = v
		}
	}
	res[idKey] = id

	return res, nil
}

// getSelectorLabel returns the selector match label
func (c *DeploymentController) getSelectorLabel(deployment *appsv1.Deployment) (string, error) {
	for _, l := range c.labels {
		if _, ok := deployment.Spec.Selector.MatchLabels[l]; ok {
			return l, nil
		}
	}

	return "", fmt.Errorf("selector not found")
}

var sidecars = map[string]bool{
	"istio-proxy": true,
	"envoy":       true,
}

func (c *DeploymentController) HaveDependenciesChanged(cd *flaggerv1.Canary) (bool, error) {
	return c.configTracker.HasConfigChanged(cd)
}

// getPorts returns a list of all container ports
func (c *DeploymentController) getPorts(cd *flaggerv1.Canary, deployment *appsv1.Deployment) (map[string]int32, error) {
	ports := make(map[string]int32)

	for _, container := range deployment.Spec.Template.Spec.Containers {
		// exclude service mesh proxies based on container name
		if _, ok := sidecars[container.Name]; ok {
			continue
		}
		for i, p := range container.Ports {
			// exclude canary.service.port or canary.service.targetPort
			if cd.Spec.Service.TargetPort.String() == "0" {
				if p.ContainerPort == cd.Spec.Service.Port {
					continue
				}
			} else {
				if cd.Spec.Service.TargetPort.Type == intstr.Int {
					if p.ContainerPort == cd.Spec.Service.TargetPort.IntVal {
						continue
					}
				}
				if cd.Spec.Service.TargetPort.Type == intstr.String {
					if p.Name == cd.Spec.Service.TargetPort.StrVal {
						continue
					}
				}
			}
			name := fmt.Sprintf("tcp-%s-%v", container.Name, i)
			if p.Name != "" {
				name = p.Name
			}

			ports[name] = p.ContainerPort
		}
	}

	return ports, nil
}

func makePrimaryLabels(labels map[string]string, primaryName string, label string) map[string]string {
	res := make(map[string]string)
	for k, v := range labels {
		if k != label {
			res[k] = v
		}
	}
	res[label] = primaryName

	return res
}

func int32p(i int32) *int32 {
	return &i
}

func int32Default(i *int32) int32 {
	if i == nil {
		return 1
	}

	return *i
}
