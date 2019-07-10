package canary

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/hashstructure"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// Deployer is managing the operations for Kubernetes deployment kind
type Deployer struct {
	KubeClient    kubernetes.Interface
	FlaggerClient clientset.Interface
	Logger        *zap.SugaredLogger
	ConfigTracker ConfigTracker
	Labels        []string
}

// Initialize creates the primary deployment, hpa,
// scales to zero the canary deployment and returns the pod selector label and container ports
func (c *Deployer) Initialize(cd *flaggerv1.Canary, skipLivenessChecks bool) (label string, ports *map[string]int32, err error) {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	label, ports, err = c.createPrimaryDeployment(cd)
	if err != nil {
		return "", ports, fmt.Errorf("creating deployment %s.%s failed: %v", primaryName, cd.Namespace, err)
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !skipLivenessChecks {
			_, readyErr := c.IsPrimaryReady(cd)
			if readyErr != nil {
				return "", ports, readyErr
			}
		}

		c.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Scaling down %s.%s", cd.Spec.TargetRef.Name, cd.Namespace)
		if err := c.Scale(cd, 0); err != nil {
			return "", ports, err
		}
	}

	if cd.Spec.AutoscalerRef != nil && cd.Spec.AutoscalerRef.Kind == "HorizontalPodAutoscaler" {
		if err := c.reconcilePrimaryHpa(cd, true); err != nil {
			return "", ports, fmt.Errorf("creating HorizontalPodAutoscaler %s.%s failed: %v", primaryName, cd.Namespace, err)
		}
	}
	return label, ports, nil
}

// Promote copies the pod spec, secrets and config maps from canary to primary
func (c *Deployer) Promote(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	canary, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
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

	primary, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", primaryName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", primaryName, cd.Namespace, err)
	}

	// promote secrets and config maps
	configRefs, err := c.ConfigTracker.GetTargetConfigs(cd)
	if err != nil {
		return err
	}
	if err := c.ConfigTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
		return err
	}

	primaryCopy := primary.DeepCopy()
	primaryCopy.Spec.ProgressDeadlineSeconds = canary.Spec.ProgressDeadlineSeconds
	primaryCopy.Spec.MinReadySeconds = canary.Spec.MinReadySeconds
	primaryCopy.Spec.RevisionHistoryLimit = canary.Spec.RevisionHistoryLimit
	primaryCopy.Spec.Strategy = canary.Spec.Strategy

	// update spec with primary secrets and config maps
	primaryCopy.Spec.Template.Spec = c.ConfigTracker.ApplyPrimaryConfigs(canary.Spec.Template.Spec, configRefs)

	// update pod annotations to ensure a rolling update
	annotations, err := c.makeAnnotations(canary.Spec.Template.Annotations)
	if err != nil {
		return err
	}
	primaryCopy.Spec.Template.Annotations = annotations

	primaryCopy.Spec.Template.Labels = makePrimaryLabels(canary.Spec.Template.Labels, primaryName, label)

	// apply update
	_, err = c.KubeClient.AppsV1().Deployments(cd.Namespace).Update(primaryCopy)
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

// HasDeploymentChanged returns true if the canary deployment pod spec has changed
func (c *Deployer) HasDeploymentChanged(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return false, fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	if cd.Status.LastAppliedSpec == "" {
		return true, nil
	}

	newHash, err := hashstructure.Hash(canary.Spec.Template, nil)
	if err != nil {
		return false, fmt.Errorf("hash error %v", err)
	}

	// do not trigger a canary deployment on manual rollback
	if cd.Status.LastPromotedSpec == fmt.Sprintf("%d", newHash) {
		return false, nil
	}

	if cd.Status.LastAppliedSpec != fmt.Sprintf("%d", newHash) {
		return true, nil
	}

	return false, nil
}

// Scale sets the canary deployment replicas
func (c *Deployer) Scale(cd *flaggerv1.Canary, replicas int32) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = int32p(replicas)

	_, err = c.KubeClient.AppsV1().Deployments(dep.Namespace).Update(depCopy)
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v failed: %v", depCopy.GetName(), depCopy.Namespace, replicas, err)
	}
	return nil
}

func (c *Deployer) createPrimaryDeployment(cd *flaggerv1.Canary) (string, *map[string]int32, error) {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDep, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
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

	var ports *map[string]int32
	if cd.Spec.Service.PortDiscovery {
		p, err := c.getPorts(canaryDep, cd.Spec.Service.Port)
		if err != nil {
			return "", nil, fmt.Errorf("port discovery failed with error: %v", err)
		}
		ports = &p
	}

	primaryDep, err := c.KubeClient.AppsV1().Deployments(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// create primary secrets and config maps
		configRefs, err := c.ConfigTracker.GetTargetConfigs(cd)
		if err != nil {
			return "", nil, err
		}
		if err := c.ConfigTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
			return "", nil, err
		}
		annotations, err := c.makeAnnotations(canaryDep.Spec.Template.Annotations)
		if err != nil {
			return "", nil, err
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
					Spec: c.ConfigTracker.ApplyPrimaryConfigs(canaryDep.Spec.Template.Spec, configRefs),
				},
			},
		}

		_, err = c.KubeClient.AppsV1().Deployments(cd.Namespace).Create(primaryDep)
		if err != nil {
			return "", nil, err
		}

		c.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("Deployment %s.%s created", primaryDep.GetName(), cd.Namespace)
	}

	return label, ports, nil
}

func (c *Deployer) reconcilePrimaryHpa(cd *flaggerv1.Canary, init bool) error {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	hpa, err := c.KubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(cd.Spec.AutoscalerRef.Name, metav1.GetOptions{})
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
	primaryHpa, err := c.KubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Get(primaryHpaName, metav1.GetOptions{})

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

		_, err = c.KubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Create(primaryHpa)
		if err != nil {
			return err
		}
		c.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("HorizontalPodAutoscaler %s.%s created", primaryHpa.GetName(), cd.Namespace)
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

			_, upErr := c.KubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(cd.Namespace).Update(hpaClone)
			if upErr != nil {
				return upErr
			}
			c.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("HorizontalPodAutoscaler %s.%s updated", primaryHpa.GetName(), cd.Namespace)
		}
	}

	return nil
}

// makeAnnotations appends an unique ID to annotations map
func (c *Deployer) makeAnnotations(annotations map[string]string) (map[string]string, error) {
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
func (c *Deployer) getSelectorLabel(deployment *appsv1.Deployment) (string, error) {
	for _, l := range c.Labels {
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

// getPorts returns a list of all container ports
func (c *Deployer) getPorts(deployment *appsv1.Deployment, canaryPort int32) (map[string]int32, error) {
	ports := make(map[string]int32)

	for _, container := range deployment.Spec.Template.Spec.Containers {
		// exclude service mesh proxies based on container name
		if _, ok := sidecars[container.Name]; ok {
			continue
		}
		for i, p := range container.Ports {
			// exclude canary.service.port
			if p.ContainerPort == canaryPort {
				continue
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
