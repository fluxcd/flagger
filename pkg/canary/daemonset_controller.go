package canary

import (
	"fmt"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

var (
	daemonSetScaleDownNodeSelector = map[string]string{"flagger.app/scale-to-zero": "true"}
)

// DaemonSetController is managing the operations for Kubernetes DaemonSet kind
type DaemonSetController struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker Tracker
	labels        []string
}

func (c *DaemonSetController) Scale(cd *flaggerv1.Canary, v int32) error {
	// there's no concept `replicas` for DaemonSet
	if v == 0 {
		targetName := cd.Spec.TargetRef.Name
		dae, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("daemonset %s.%s not found", targetName, cd.Namespace)
			}
			return fmt.Errorf("daemonset %s.%s query error %v", targetName, cd.Namespace, err)
		}

		daeCopy := dae.DeepCopy()
		daeCopy.Spec.Template.Spec.NodeSelector = make(map[string]string,
			len(dae.Spec.Template.Spec.NodeSelector)+len(daemonSetScaleDownNodeSelector))
		for k, v := range dae.Spec.Template.Spec.NodeSelector {
			daeCopy.Spec.Template.Spec.NodeSelector[k] = v
		}

		for k, v := range daemonSetScaleDownNodeSelector {
			daeCopy.Spec.Template.Spec.NodeSelector[k] = v
		}

		_, err = c.kubeClient.AppsV1().DaemonSets(dae.Namespace).Update(daeCopy)
		if err != nil {
			return fmt.Errorf("scaling down daemonset %s.%s failed: %v", daeCopy.GetName(), daeCopy.Namespace, err)
		}
	}
	return nil
}

func (c *DaemonSetController) ScaleFromZero(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	dep, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("daemonset %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("daemonset %s.%s query error %v", targetName, cd.Namespace, err)
	}

	depCopy := dep.DeepCopy()
	for k := range daemonSetScaleDownNodeSelector {
		delete(depCopy.Spec.Template.Spec.NodeSelector, k)
	}

	_, err = c.kubeClient.AppsV1().DaemonSets(dep.Namespace).Update(depCopy)
	if err != nil {
		return fmt.Errorf("scaling up daemonset %s.%s failed: %v", depCopy.GetName(), depCopy.Namespace, err)
	}
	return nil
}

// Initialize creates the primary DaemonSet and
// delete the canary DaemonSet and returns the pod selector label and container ports
func (c *DaemonSetController) Initialize(cd *flaggerv1.Canary, skipLivenessChecks bool) (err error) {
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)
	err = c.createPrimaryDaemonSet(cd)
	if err != nil {
		return fmt.Errorf("creating daemonset %s.%s failed: %v", primaryName, cd.Namespace, err)
	}

	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !skipLivenessChecks && !cd.SkipAnalysis() {
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
	return nil
}

// Promote copies the pod spec, secrets and config maps from canary to primary
func (c *DaemonSetController) Promote(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", targetName)

	canary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("damonset %s.%s not found", targetName, cd.Namespace)
		}
		return fmt.Errorf("damonset %s.%s query error %v", targetName, cd.Namespace, err)
	}

	label, err := c.getSelectorLabel(canary)
	if err != nil {
		return fmt.Errorf("invalid label selector! DaemonSet %s.%s spec.selector.matchLabels must contain selector 'app: %s'",
			targetName, cd.Namespace, targetName)
	}

	primary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("daemonset %s.%s not found", primaryName, cd.Namespace)
		}
		return fmt.Errorf("daemonset %s.%s query error %v", primaryName, cd.Namespace, err)
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
	primaryCopy.Spec.MinReadySeconds = canary.Spec.MinReadySeconds
	primaryCopy.Spec.RevisionHistoryLimit = canary.Spec.RevisionHistoryLimit
	primaryCopy.Spec.UpdateStrategy = canary.Spec.UpdateStrategy

	// update spec with primary secrets and config maps
	primaryCopy.Spec.Template.Spec = c.configTracker.ApplyPrimaryConfigs(canary.Spec.Template.Spec, configRefs)

	// ignore `daemonSetScaleDownNodeSelector` node selector
	for key := range daemonSetScaleDownNodeSelector {
		delete(primaryCopy.Spec.Template.Spec.NodeSelector, key)
	}

	// update pod annotations to ensure a rolling update
	annotations, err := makeAnnotations(canary.Spec.Template.Annotations)
	if err != nil {
		return err
	}
	primaryCopy.Spec.Template.Annotations = annotations

	primaryCopy.Spec.Template.Labels = makePrimaryLabels(canary.Spec.Template.Labels, primaryName, label)

	// apply update
	_, err = c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Update(primaryCopy)
	if err != nil {
		return fmt.Errorf("updating deployment %s.%s template spec failed: %v",
			primaryCopy.GetName(), primaryCopy.Namespace, err)
	}
	return nil
}

// HasTargetChanged returns true if the canary DaemonSet pod spec has changed
func (c *DaemonSetController) HasTargetChanged(cd *flaggerv1.Canary) (bool, error) {
	targetName := cd.Spec.TargetRef.Name
	canary, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, fmt.Errorf("daemonset %s.%s not found", targetName, cd.Namespace)
		}
		return false, fmt.Errorf("daemonset %s.%s query error %v", targetName, cd.Namespace, err)
	}

	// ignore `daemonSetScaleDownNodeSelector` node selector
	for key := range daemonSetScaleDownNodeSelector {
		delete(canary.Spec.Template.Spec.NodeSelector, key)
	}

	// since nil and capacity zero map would have different hash, we have to initialize here
	if canary.Spec.Template.Spec.NodeSelector == nil {
		canary.Spec.Template.Spec.NodeSelector = map[string]string{}
	}

	return hasSpecChanged(cd, canary.Spec.Template)
}

// GetMetadata returns the pod label selector and svc ports
func (c *DaemonSetController) GetMetadata(cd *flaggerv1.Canary) (string, map[string]int32, error) {
	targetName := cd.Spec.TargetRef.Name

	canaryDae, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil, fmt.Errorf("daemonset %s.%s not found, retrying", targetName, cd.Namespace)
		}
		return "", nil, err
	}

	label, err := c.getSelectorLabel(canaryDae)
	if err != nil {
		return "", nil, fmt.Errorf("invalid label selector! DaemonSet %s.%s spec.selector.matchLabels must contain selector 'app: %s'",
			targetName, cd.Namespace, targetName)
	}

	var ports map[string]int32
	if cd.Spec.Service.PortDiscovery {
		p, err := getPorts(cd, canaryDae.Spec.Template.Spec.Containers)
		if err != nil {
			return "", nil, fmt.Errorf("port discovery failed with error: %v", err)
		}
		ports = p
	}

	return label, ports, nil
}

func (c *DaemonSetController) createPrimaryDaemonSet(cd *flaggerv1.Canary) error {
	targetName := cd.Spec.TargetRef.Name
	primaryName := fmt.Sprintf("%s-primary", cd.Spec.TargetRef.Name)

	canaryDae, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("daemonset %s.%s not found, retrying", targetName, cd.Namespace)
		}
		return err
	}

	if canaryDae.Spec.UpdateStrategy.Type != "" &&
		canaryDae.Spec.UpdateStrategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return fmt.Errorf("daemonset %s.%s must have RollingUpdate strategy but have %s",
			targetName, cd.Namespace, canaryDae.Spec.UpdateStrategy.Type)
	}

	label, err := c.getSelectorLabel(canaryDae)
	if err != nil {
		return fmt.Errorf("invalid label selector! DaemonSet %s.%s spec.selector.matchLabels must contain selector 'app: %s'",
			targetName, cd.Namespace, targetName)
	}

	primaryDep, err := c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Get(primaryName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// create primary secrets and config maps
		configRefs, err := c.configTracker.GetTargetConfigs(cd)
		if err != nil {
			return err
		}
		if err := c.configTracker.CreatePrimaryConfigs(cd, configRefs); err != nil {
			return err
		}
		annotations, err := makeAnnotations(canaryDae.Spec.Template.Annotations)
		if err != nil {
			return err
		}

		// create primary deployment
		primaryDep = &appsv1.DaemonSet{
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
			Spec: appsv1.DaemonSetSpec{
				MinReadySeconds:      canaryDae.Spec.MinReadySeconds,
				RevisionHistoryLimit: canaryDae.Spec.RevisionHistoryLimit,
				UpdateStrategy:       canaryDae.Spec.UpdateStrategy,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						label: primaryName,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      makePrimaryLabels(canaryDae.Spec.Template.Labels, primaryName, label),
						Annotations: annotations,
					},
					// update spec with the primary secrets and config maps
					Spec: c.configTracker.ApplyPrimaryConfigs(canaryDae.Spec.Template.Spec, configRefs),
				},
			},
		}

		_, err = c.kubeClient.AppsV1().DaemonSets(cd.Namespace).Create(primaryDep)
		if err != nil {
			return err
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).Infof("DaemonSet %s.%s created", primaryDep.GetName(), cd.Namespace)
	}
	return nil
}

// getSelectorLabel returns the selector match label
func (c *DaemonSetController) getSelectorLabel(daemonSet *appsv1.DaemonSet) (string, error) {
	for _, l := range c.labels {
		if _, ok := daemonSet.Spec.Selector.MatchLabels[l]; ok {
			return l, nil
		}
	}

	return "", fmt.Errorf("selector not found")
}

func (c *DaemonSetController) HaveDependenciesChanged(cd *flaggerv1.Canary) (bool, error) {
	return c.configTracker.HasConfigChanged(cd)
}
