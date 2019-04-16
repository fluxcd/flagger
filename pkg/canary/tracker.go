package canary

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
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

// getRefFromConfigMap transforms a Kubernetes ConfigMap into a ConfigRef
// and computes the checksum of the ConfigMap data
func (ct *ConfigTracker) getRefFromConfigMap(name string, namespace string) (*ConfigRef, error) {
	config, err := ct.KubeClient.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
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
	secret, err := ct.KubeClient.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// ignore registry secrets (those should be set via service account)
	if secret.Type != corev1.SecretTypeOpaque &&
		secret.Type != corev1.SecretTypeBasicAuth &&
		secret.Type != corev1.SecretTypeSSHAuth &&
		secret.Type != corev1.SecretTypeTLS {
		ct.Logger.Debugf("ignoring secret %s.%s type not supported %v", name, namespace, secret.Type)
		return nil, nil
	}

	return &ConfigRef{
		Name:     secret.Name,
		Type:     ConfigRefSecret,
		Checksum: checksum(secret.Data),
	}, nil
}

// GetTargetConfigs scans the target deployment for Kubernetes ConfigMaps and Secretes
// and returns a list of config references
func (ct *ConfigTracker) GetTargetConfigs(cd *flaggerv1.Canary) (map[string]ConfigRef, error) {
	res := make(map[string]ConfigRef)
	targetName := cd.Spec.TargetRef.Name
	targetDep, err := ct.KubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return res, fmt.Errorf("deployment %s.%s not found", targetName, cd.Namespace)
		}
		return res, fmt.Errorf("deployment %s.%s query error %v", targetName, cd.Namespace, err)
	}

	// scan volumes
	for _, volume := range targetDep.Spec.Template.Spec.Volumes {
		if cmv := volume.ConfigMap; cmv != nil {
			config, err := ct.getRefFromConfigMap(cmv.Name, cd.Namespace)
			if err != nil {
				ct.Logger.Errorf("configMap %s.%s query error %v", cmv.Name, cd.Namespace, err)
				continue
			}
			if config != nil {
				res[config.GetName()] = *config
			}
		}

		if sv := volume.Secret; sv != nil {
			secret, err := ct.getRefFromSecret(sv.SecretName, cd.Namespace)
			if err != nil {
				ct.Logger.Errorf("secret %s.%s query error %v", sv.SecretName, cd.Namespace, err)
				continue
			}
			if secret != nil {
				res[secret.GetName()] = *secret
			}
		}
	}
	// scan containers
	for _, container := range targetDep.Spec.Template.Spec.Containers {
		// scan env
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				switch {
				case env.ValueFrom.ConfigMapKeyRef != nil:
					name := env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name
					config, err := ct.getRefFromConfigMap(name, cd.Namespace)
					if err != nil {
						ct.Logger.Errorf("configMap %s.%s query error %v", name, cd.Namespace, err)
						continue
					}
					if config != nil {
						res[config.GetName()] = *config
					}
				case env.ValueFrom.SecretKeyRef != nil:
					name := env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					secret, err := ct.getRefFromSecret(name, cd.Namespace)
					if err != nil {
						ct.Logger.Errorf("secret %s.%s query error %v", name, cd.Namespace, err)
						continue
					}
					if secret != nil {
						res[secret.GetName()] = *secret
					}
				}
			}
		}
		// scan envFrom
		for _, envFrom := range container.EnvFrom {
			switch {
			case envFrom.ConfigMapRef != nil:
				name := envFrom.ConfigMapRef.LocalObjectReference.Name
				config, err := ct.getRefFromConfigMap(name, cd.Namespace)
				if err != nil {
					ct.Logger.Errorf("configMap %s.%s query error %v", name, cd.Namespace, err)
					continue
				}
				if config != nil {
					res[config.GetName()] = *config
				}
			case envFrom.SecretRef != nil:
				name := envFrom.SecretRef.LocalObjectReference.Name
				secret, err := ct.getRefFromSecret(name, cd.Namespace)
				if err != nil {
					ct.Logger.Errorf("secret %s.%s query error %v", name, cd.Namespace, err)
					continue
				}
				if secret != nil {
					res[secret.GetName()] = *secret
				}
			}
		}
	}

	return res, nil
}

// GetConfigRefs returns a map of configs and their checksum
func (ct *ConfigTracker) GetConfigRefs(cd *flaggerv1.Canary) (*map[string]string, error) {
	res := make(map[string]string)
	configs, err := ct.GetTargetConfigs(cd)
	if err != nil {
		return nil, err
	}

	for _, cfg := range configs {
		res[cfg.GetName()] = cfg.Checksum
	}

	return &res, nil
}

// HasConfigChanged checks for changes in ConfigMaps and Secretes by comparing
// the checksum for each ConfigRef stored in Canary.Status.TrackedConfigs
func (ct *ConfigTracker) HasConfigChanged(cd *flaggerv1.Canary) (bool, error) {
	configs, err := ct.GetTargetConfigs(cd)
	if err != nil {
		return false, err
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

// CreatePrimaryConfigs syncs the primary Kubernetes ConfigMaps and Secretes
// with those found in the target deployment
func (ct *ConfigTracker) CreatePrimaryConfigs(cd *flaggerv1.Canary, refs map[string]ConfigRef) error {
	for _, ref := range refs {
		switch ref.Type {
		case ConfigRefMap:
			config, err := ct.KubeClient.CoreV1().ConfigMaps(cd.Namespace).Get(ref.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			primaryName := fmt.Sprintf("%s-primary", config.GetName())
			primaryConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      primaryName,
					Namespace: cd.Namespace,
					Labels:    config.Labels,
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
			_, err = ct.KubeClient.CoreV1().ConfigMaps(cd.Namespace).Update(primaryConfigMap)
			if err != nil {
				if errors.IsNotFound(err) {
					_, err = ct.KubeClient.CoreV1().ConfigMaps(cd.Namespace).Create(primaryConfigMap)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}

			ct.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("ConfigMap %s synced", primaryConfigMap.GetName())
		case ConfigRefSecret:
			secret, err := ct.KubeClient.CoreV1().Secrets(cd.Namespace).Get(ref.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			primaryName := fmt.Sprintf("%s-primary", secret.GetName())
			primarySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      primaryName,
					Namespace: cd.Namespace,
					Labels:    secret.Labels,
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
			_, err = ct.KubeClient.CoreV1().Secrets(cd.Namespace).Update(primarySecret)
			if err != nil {
				if errors.IsNotFound(err) {
					_, err = ct.KubeClient.CoreV1().Secrets(cd.Namespace).Create(primarySecret)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}

			ct.Logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace)).
				Infof("Secret %s synced", primarySecret.GetName())
		}
	}

	return nil
}

// ApplyPrimaryConfigs appends the primary suffix to all ConfigMaps and Secretes found in the PodSpec
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
