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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// ConfigTracker is managing the operations for Kubernetes ConfigMaps and Secrets
type ConfigTracker struct {
	KubeClient    kubernetes.Interface
	FlaggerClient clientset.Interface
	Logger        *zap.SugaredLogger
}

type ConfigRefType string

const (
	ConfigRefMap    ConfigRefType = "configmap"
	ConfigRefSecret ConfigRefType = "secret"

	configTrackingDisabledAnnotationKey = "flagger.app/config-tracking"
)

// ConfigRef holds the reference to a tracked Kubernetes ConfigMap or Secret
type ConfigRef struct {
	Name     string
	Type     ConfigRefType
	Checksum string
}

// GetName returns the config ref type and name
func (c *ConfigRef) GetName() string {
	return fmt.Sprintf("%s/%s", c.Type, c.Name)
}

func checksum(data interface{}) string {
	jsonBytes, _ := json.Marshal(data)
	hashBytes := sha256.Sum256(jsonBytes)

	return fmt.Sprintf("%x", hashBytes[:8])
}

func configIsDisabled(annotations map[string]string) bool {
	return strings.HasPrefix(annotations[configTrackingDisabledAnnotationKey], "disable")
}

// getRefFromConfigMap transforms a Kubernetes ConfigMap into a ConfigRef
// and computes the checksum of the ConfigMap data
func (ct *ConfigTracker) getRefFromConfigMap(name string, namespace string) (*ConfigRef, error) {
	config, err := ct.KubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("configmap  %s.%s get query error: %w", name, namespace, err)
	}

	if configIsDisabled(config.GetAnnotations()) {
		return nil, nil
	}

	return &ConfigRef{
		Name:     config.Name,
		Type:     ConfigRefMap,
		Checksum: checksum(config.Data),
	}, nil
}

// getRefFromConfigMap transforms a Kubernetes Secret into a ConfigRef
// and computes the checksum of the Secret data
func (ct *ConfigTracker) getRefFromSecret(name string, namespace string) (*ConfigRef, error) {
	secret, err := ct.KubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("secret %s.%s get query error: %w", name, namespace, err)
	}

	// ignore registry secrets (those should be set via service account)
	if secret.Type != corev1.SecretTypeOpaque &&
		secret.Type != corev1.SecretTypeBasicAuth &&
		secret.Type != corev1.SecretTypeSSHAuth &&
		secret.Type != corev1.SecretTypeTLS {
		ct.Logger.Debugf("ignoring secret %s.%s type not supported %v", name, namespace, secret.Type)
		return nil, nil
	}

	if configIsDisabled(secret.GetAnnotations()) {
		return nil, nil
	}

	return &ConfigRef{
		Name:     secret.Name,
		Type:     ConfigRefSecret,
		Checksum: checksum(secret.Data),
	}, nil
}

// GetTargetConfigs scans the target deployment for Kubernetes ConfigMaps and Secrets
// and returns a list of config references
func (ct *ConfigTracker) GetTargetConfigs(cd *flaggerv1.Canary) (map[string]ConfigRef, error) {
	targetName := cd.Spec.TargetRef.Name
	var vs []corev1.Volume
	var cs []corev1.Container

	switch cd.Spec.TargetRef.Kind {
	case "Deployment":
		targetDep, err := ct.KubeClient.AppsV1().Deployments(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("deployment %s.%s get query error: %w", targetName, cd.Namespace, err)
		}
		vs = targetDep.Spec.Template.Spec.Volumes
		cs = targetDep.Spec.Template.Spec.Containers
		cs = append(cs, targetDep.Spec.Template.Spec.InitContainers...)
	case "DaemonSet":
		targetDae, err := ct.KubeClient.AppsV1().DaemonSets(cd.Namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("daemonset %s.%s get query error: %w", targetName, cd.Namespace, err)
		}
		vs = targetDae.Spec.Template.Spec.Volumes
		cs = targetDae.Spec.Template.Spec.Containers
		cs = append(cs, targetDae.Spec.Template.Spec.InitContainers...)
	default:
		return nil, fmt.Errorf("TargetRef.Kind invalid: %s", cd.Spec.TargetRef.Kind)
	}

	secretNames := make(map[string]bool)
	configMapNames := make(map[string]bool)

	// scan volumes
	for _, volume := range vs {
		if cmv := volume.ConfigMap; cmv != nil {
			name := cmv.Name
			configMapNames[name] = fieldIsMandatory(cmv.Optional)
		}
		if sv := volume.Secret; sv != nil {
			name := sv.SecretName
			secretNames[name] = fieldIsMandatory(sv.Optional)
		}

		if projected := volume.Projected; projected != nil {
			for _, source := range projected.Sources {
				if cmv := source.ConfigMap; cmv != nil {
					name := cmv.Name
					configMapNames[name] = fieldIsMandatory(cmv.Optional)
				}
				if sv := source.Secret; sv != nil {
					name := sv.Name
					secretNames[name] = fieldIsMandatory(sv.Optional)
				}
			}
		}
	}
	// scan containers
	for _, container := range cs {
		// scan env
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				switch {
				case env.ValueFrom.ConfigMapKeyRef != nil:
					name := env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name
					configMapNames[name] = fieldIsMandatory(env.ValueFrom.ConfigMapKeyRef.Optional)
				case env.ValueFrom.SecretKeyRef != nil:
					name := env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					secretNames[name] = fieldIsMandatory(env.ValueFrom.SecretKeyRef.Optional)
				}
			}
		}
		// scan envFrom
		for _, envFrom := range container.EnvFrom {
			switch {
			case envFrom.ConfigMapRef != nil:
				name := envFrom.ConfigMapRef.LocalObjectReference.Name
				configMapNames[name] = fieldIsMandatory(envFrom.ConfigMapRef.Optional)
			case envFrom.SecretRef != nil:
				name := envFrom.SecretRef.LocalObjectReference.Name
				secretNames[name] = fieldIsMandatory(envFrom.SecretRef.Optional)
			}
		}
	}

	res := make(map[string]ConfigRef)

	for configMapName, required := range configMapNames {
		config, err := ct.getRefFromConfigMap(configMapName, cd.Namespace)
		if err != nil {
			if required {
				return nil, fmt.Errorf("configmap %s.%s get query error: %w", configMapName, cd.Namespace, err)
			}
			ct.Logger.Errorf("configmap %s.%s get query failed: %w", configMapName, cd.Namespace, err)
			continue
		}
		if config != nil {
			res[config.GetName()] = *config
		}
	}

	for secretName, required := range secretNames {
		secret, err := ct.getRefFromSecret(secretName, cd.Namespace)
		if err != nil {
			if required {
				return nil, fmt.Errorf("secret %s.%s get query error: %v", secretName, cd.Namespace, err)
			}
			ct.Logger.Errorf("secret %s.%s get query failed: %v", secretName, cd.Namespace, err)
			continue
		}
		if secret != nil {
			res[secret.GetName()] = *secret
		}
	}

	return res, nil
}

func fieldIsMandatory(p *bool) bool {
	if p == nil {
		return false
	}
	return !*p
}

// GetConfigRefs returns a map of configs and their checksum
func (ct *ConfigTracker) GetConfigRefs(cd *flaggerv1.Canary) (*map[string]string, error) {
	res := make(map[string]string)
	configs, err := ct.GetTargetConfigs(cd)
	if err != nil {
		return nil, fmt.Errorf("GetTargetConfigs failed: %w", err)
	}

	for _, cfg := range configs {
		res[cfg.GetName()] = cfg.Checksum
	}

	return &res, nil
}

// HasConfigChanged checks for changes in ConfigMaps and Secrets by comparing
// the checksum for each ConfigRef stored in Canary.Status.TrackedConfigs
func (ct *ConfigTracker) HasConfigChanged(cd *flaggerv1.Canary) (bool, error) {
	configs, err := ct.GetTargetConfigs(cd)
	if err != nil {
		return false, fmt.Errorf("GetTargetConfigs failed: %w", err)
	}

	if len(configs) == 0 && cd.Status.TrackedConfigs == nil {
		return false, nil
	}

	if len(configs) > 0 && cd.Status.TrackedConfigs == nil {
		return true, nil
	}

	trackedConfigs := *cd.Status.TrackedConfigs

	if len(configs) != len(trackedConfigs) {
		return true, nil
	}

	for _, cfg := range configs {
		if trackedConfigs[cfg.GetName()] != cfg.Checksum {
			ct.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("%s %s has changed", cfg.Type, cfg.Name)
			return true, nil
		}
	}

	return false, nil
}

// CreatePrimaryConfigs syncs the primary Kubernetes ConfigMaps and Secrets
// with those found in the target deployment
func (ct *ConfigTracker) CreatePrimaryConfigs(cd *flaggerv1.Canary, refs map[string]ConfigRef, includeLabelPrefix []string) error {
	for _, ref := range refs {
		switch ref.Type {
		case ConfigRefMap:
			config, err := ct.KubeClient.CoreV1().ConfigMaps(cd.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("configmap %s.%s get query failed : %w", ref.Name, cd.Name, err)
			}
			primaryName := fmt.Sprintf("%s-primary", config.GetName())
			labels := includeLabelsByPrefix(config.Labels, includeLabelPrefix)
			primaryConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      primaryName,
					Namespace: cd.Namespace,
					Labels:    labels,
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(cd, schema.GroupVersionKind{
							Group:   flaggerv1.SchemeGroupVersion.Group,
							Version: flaggerv1.SchemeGroupVersion.Version,
							Kind:    flaggerv1.CanaryKind,
						}),
					},
				},
				Data: config.Data,
			}

			// update or insert primary ConfigMap
			_, err = ct.KubeClient.CoreV1().ConfigMaps(cd.Namespace).Update(context.TODO(), primaryConfigMap, metav1.UpdateOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					_, err = ct.KubeClient.CoreV1().ConfigMaps(cd.Namespace).Create(context.TODO(), primaryConfigMap, metav1.CreateOptions{})
					if err != nil {
						return fmt.Errorf("creating configmap %s.%s failed: %w", primaryConfigMap.Name, cd.Namespace, err)
					}
				} else {
					return fmt.Errorf("updating configmap %s.%s failed: %w", primaryConfigMap.Name, cd.Namespace, err)
				}
			}

			ct.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("ConfigMap %s synced", primaryConfigMap.GetName())
		case ConfigRefSecret:
			secret, err := ct.KubeClient.CoreV1().Secrets(cd.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("secret %s.%s get query failed : %w", ref.Name, cd.Name, err)
			}
			primaryName := fmt.Sprintf("%s-primary", secret.GetName())
			labels := includeLabelsByPrefix(secret.Labels, includeLabelPrefix)
			primarySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      primaryName,
					Namespace: cd.Namespace,
					Labels:    labels,
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(cd, schema.GroupVersionKind{
							Group:   flaggerv1.SchemeGroupVersion.Group,
							Version: flaggerv1.SchemeGroupVersion.Version,
							Kind:    flaggerv1.CanaryKind,
						}),
					},
				},
				Type: secret.Type,
				Data: secret.Data,
			}

			// update or insert primary Secret
			_, err = ct.KubeClient.CoreV1().Secrets(cd.Namespace).Update(context.TODO(), primarySecret, metav1.UpdateOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					_, err = ct.KubeClient.CoreV1().Secrets(cd.Namespace).Create(context.TODO(), primarySecret, metav1.CreateOptions{})
					if err != nil {
						return fmt.Errorf("creating secret %s.%s failed: %w", primarySecret.Name, cd.Namespace, err)
					}
				} else {
					return fmt.Errorf("updating secret %s.%s failed: %w", primarySecret.Name, cd.Namespace, err)
				}
			}

			ct.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("Secret %s synced", primarySecret.GetName())
		}
	}

	return nil
}

// ApplyPrimaryConfigs appends the primary suffix to all ConfigMaps and Secrets found in the PodSpec
func (ct *ConfigTracker) ApplyPrimaryConfigs(spec corev1.PodSpec, refs map[string]ConfigRef) corev1.PodSpec {
	// update volumes
	for i, volume := range spec.Volumes {
		if cmv := volume.ConfigMap; cmv != nil {
			name := fmt.Sprintf("%s/%s", ConfigRefMap, cmv.Name)
			if _, exists := refs[name]; exists {
				spec.Volumes[i].ConfigMap.Name += "-primary"
			}
		}

		if sv := volume.Secret; sv != nil {
			name := fmt.Sprintf("%s/%s", ConfigRefSecret, sv.SecretName)
			if _, exists := refs[name]; exists {
				spec.Volumes[i].Secret.SecretName += "-primary"
			}
		}

		if projected := volume.Projected; projected != nil {
			for s, source := range projected.Sources {
				if cmv := source.ConfigMap; cmv != nil {
					name := fmt.Sprintf("%s/%s", ConfigRefMap, cmv.Name)
					if _, exists := refs[name]; exists {
						spec.Volumes[i].Projected.Sources[s].ConfigMap.Name += "-primary"
					}
				}

				if sv := source.Secret; sv != nil {
					name := fmt.Sprintf("%s/%s", ConfigRefSecret, sv.Name)
					if _, exists := refs[name]; exists {
						spec.Volumes[i].Projected.Sources[s].Secret.Name += "-primary"
					}
				}
			}
		}
	}

	// update containers
	for _, container := range spec.Containers {
		// update env
		for i, env := range container.Env {
			if env.ValueFrom != nil {
				switch {
				case env.ValueFrom.ConfigMapKeyRef != nil:
					name := fmt.Sprintf("%s/%s", ConfigRefMap, env.ValueFrom.ConfigMapKeyRef.Name)
					if _, exists := refs[name]; exists {
						container.Env[i].ValueFrom.ConfigMapKeyRef.Name += "-primary"
					}
				case env.ValueFrom.SecretKeyRef != nil:
					name := fmt.Sprintf("%s/%s", ConfigRefSecret, env.ValueFrom.SecretKeyRef.Name)
					if _, exists := refs[name]; exists {
						container.Env[i].ValueFrom.SecretKeyRef.Name += "-primary"
					}
				}
			}
		}
		// update envFrom
		for i, envFrom := range container.EnvFrom {
			switch {
			case envFrom.ConfigMapRef != nil:
				name := fmt.Sprintf("%s/%s", ConfigRefMap, envFrom.ConfigMapRef.Name)
				if _, exists := refs[name]; exists {
					container.EnvFrom[i].ConfigMapRef.Name += "-primary"
				}
			case envFrom.SecretRef != nil:
				name := fmt.Sprintf("%s/%s", ConfigRefSecret, envFrom.SecretRef.Name)
				if _, exists := refs[name]; exists {
					container.EnvFrom[i].SecretRef.Name += "-primary"
				}
			}
		}
	}

	return spec
}
