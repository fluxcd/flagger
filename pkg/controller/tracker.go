package controller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ConfigTracker is managing the operations for Kubernetes configmaps and secrets
type ConfigTracker struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

type ConfigRefType string

const (
	ConfigRefMap    ConfigRefType = "configmap"
	ConfigRefSecret ConfigRefType = "secret"
)

// ConfigRef holds the reference to a tracked configmap or secrets
type ConfigRef struct {
	Name     string
	Type     ConfigRefType
	Checksum string
}

func (c *ConfigRef) GetName() string {
	return fmt.Sprintf("%s/%s", c.Type, c.Name)
}

func checksum(data interface{}) string {
	jsonBytes, _ := json.Marshal(data)
	hashBytes := sha256.Sum256(jsonBytes)
	return fmt.Sprintf("%x", hashBytes[:8])
}

func (ct *ConfigTracker) getTargetConfigs(cd *flaggerv1.Canary) (map[string]ConfigRef, error) {
	res := make(map[string]ConfigRef)
	targetName := cd.Spec.TargetRef.Name
	targetDep, err := ct.kubeClient.AppsV1().Deployments(cd.Namespace).Get(targetName, metav1.GetOptions{})
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
				ct.logger.Errorf("configMap %s.%s query error %v", cmv.Name, cd.Namespace, err)
				continue
			}
			if config != nil {
				res[config.GetName()] = *config
			}
		}

		if sv := volume.Secret; sv != nil {
			secret, err := ct.getRefFromSecret(sv.SecretName, cd.Namespace)
			if err != nil {
				ct.logger.Errorf("secret %s.%s query error %v", sv.SecretName, cd.Namespace, err)
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
						ct.logger.Errorf("configMap %s.%s query error %v", name, cd.Namespace, err)
						continue
					}
					if config != nil {
						res[config.GetName()] = *config
					}
				case env.ValueFrom.SecretKeyRef != nil:
					name := env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					secret, err := ct.getRefFromSecret(name, cd.Namespace)
					if err != nil {
						ct.logger.Errorf("secret %s.%s query error %v", name, cd.Namespace, err)
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
					ct.logger.Errorf("configMap %s.%s query error %v", name, cd.Namespace, err)
					continue
				}
				if config != nil {
					res[config.GetName()] = *config
				}
			case envFrom.SecretRef != nil:
				name := envFrom.SecretRef.LocalObjectReference.Name
				secret, err := ct.getRefFromSecret(name, cd.Namespace)
				if err != nil {
					ct.logger.Errorf("secret %s.%s query error %v", name, cd.Namespace, err)
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

func (ct *ConfigTracker) getRefFromConfigMap(name string, namespace string) (*ConfigRef, error) {
	config, err := ct.kubeClient.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &ConfigRef{
		Name:     config.Name,
		Type:     ConfigRefMap,
		Checksum: checksum(config.Data),
	}, nil
}

func (ct *ConfigTracker) getRefFromSecret(name string, namespace string) (*ConfigRef, error) {
	secret, err := ct.kubeClient.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if secret.Type != corev1.SecretTypeOpaque &&
		secret.Type != corev1.SecretTypeBasicAuth &&
		secret.Type != corev1.SecretTypeSSHAuth &&
		secret.Type != corev1.SecretTypeTLS {
		ct.logger.Debugf("ignoring secret %s.%s type not supported %v", name, namespace, secret.Type)
		return nil, nil
	}

	return &ConfigRef{
		Name:     secret.Name,
		Type:     ConfigRefSecret,
		Checksum: checksum(secret.Data),
	}, nil
}
