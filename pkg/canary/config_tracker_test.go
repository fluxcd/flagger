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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTesting "k8s.io/client-go/testing"
)

func TestConfigIsDisabled(t *testing.T) {
	for _, c := range []struct {
		annotations map[string]string
		exp         bool
	}{
		{annotations: map[string]string{configTrackingDisabledAnnotationKey: "disable"}, exp: true},
		{annotations: map[string]string{"app": "disable"}, exp: false},
		{annotations: map[string]string{}, exp: false},
	} {
		assert.Equal(t, configIsDisabled(c.annotations), c.exp)
	}
}

func TestConfigTracker_ConfigMaps(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDeploymentFixture(dc)
		configMap := newDeploymentControllerTestConfigMap()
		configMapProjected := newDeploymentControllerTestConfigProjected()

		mocks.initializeCanary(t)

		depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
		require.NoError(t, err)

		configPrimaryVolName := depPrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
		assert.Equal(t, "podinfo-config-vol-primary", configPrimaryVolName)

		configPrimaryInit, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-init-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryInit.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryInit.BinaryData["color_binary"])
		}

		configPrimaryInitEnv, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-init-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryInitEnv.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryInitEnv.BinaryData["color_binary"])
		}

		configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimary.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimary.BinaryData["color_binary"])
		}

		configPrimaryEnv, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryEnv.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryEnv.BinaryData["color_binary"])
		}

		configPrimaryVol, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-vol-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryVol.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryVol.BinaryData["color_binary"])
		}

		configProjectedName := depPrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[0].ConfigMap.Name
		assert.Equal(t, "podinfo-config-projected-primary", configProjectedName)

		configPrimaryProjected, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-vol-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMapProjected.Data["color"], configPrimaryProjected.Data["color"])
		}

		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-enabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-enabled-primary", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-disabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-disabled-primary", metav1.GetOptions{})
		assert.Error(t, err)

		var trackedVolPresent, originalVolPresent bool
		for _, vol := range depPrimary.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil {
				switch vol.ConfigMap.Name {
				case "podinfo-config-tracker-enabled":
					assert.Fail(t, "primary Deployment does not contain a volume for config-tracked configmap %q", vol.ConfigMap.Name)
				case "podinfo-config-tracker-enabled-primary":
					trackedVolPresent = true
				case "podinfo-config-tracker-disabled":
					originalVolPresent = true
				case "podinfo-config-tracker-disabled-primary":
					assert.Fail(t, "primary Deployment incorrectly contains a volume for a copy of an untracked configmap %q", vol.ConfigMap.Name)
				}
			}
		}
		assert.True(t, trackedVolPresent, "Volume for primary copy of config-tracked configmap should be present")
		assert.True(t, originalVolPresent, "Volume for original configmap with disabled tracking should be present")
	})

	t.Run("daemonset", func(t *testing.T) {
		dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDaemonSetFixture(dc)
		configMap := newDaemonSetControllerTestConfigMap()
		configMapProjected := newDaemonSetControllerTestConfigProjected()

		_, err := mocks.controller.Initialize(mocks.canary)
		require.NoError(t, err)

		daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
		require.NoError(t, err)

		configPrimaryVolName := daePrimary.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name
		assert.Equal(t, "podinfo-config-vol-primary", configPrimaryVolName)

		configPrimaryInit, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-init-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryInit.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryInit.BinaryData["color_binary"])
		}

		configPrimaryInitEnv, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-init-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryInitEnv.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryInitEnv.BinaryData["color_binary"])
		}

		configPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimary.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimary.BinaryData["color_binary"])
		}

		configPrimaryEnv, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryEnv.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryEnv.BinaryData["color_binary"])
		}

		configPrimaryVol, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-vol-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMap.Data["color"], configPrimaryVol.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryVol.BinaryData["color_binary"])
		}

		configProjectedName := daePrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[0].ConfigMap.Name
		assert.Equal(t, "podinfo-config-projected-primary", configProjectedName)

		configPrimaryProjected, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-vol-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, configMapProjected.Data["color"], configPrimaryProjected.Data["color"])
			assert.Equal(t, configMap.BinaryData["color_binary"], configPrimaryProjected.BinaryData["color_binary"])
		}

		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-enabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-enabled-primary", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-disabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-tracker-disabled-primary", metav1.GetOptions{})
		assert.Error(t, err)

		var trackedVolPresent, originalVolPresent bool
		for _, vol := range daePrimary.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil {
				switch vol.ConfigMap.Name {
				case "podinfo-config-tracker-enabled":
					assert.Fail(t, "primary Deployment does not contain a volume for config-tracked configmap %q", vol.ConfigMap.Name)
				case "podinfo-config-tracker-enabled-primary":
					trackedVolPresent = true
				case "podinfo-config-tracker-disabled":
					originalVolPresent = true
				case "podinfo-config-tracker-disabled-primary":
					assert.Fail(t, "primary Deployment incorrectly contains a volume for a copy of an untracked configmap %q", vol.ConfigMap.Name)
				}
			}
		}
		assert.True(t, trackedVolPresent, "Volume for primary copy of config-tracked configmap should be present")
		assert.True(t, originalVolPresent, "Volume for original configmap with disabled tracking should be present")
	})
}

func TestConfigTracker_Secrets(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDeploymentFixture(dc)
		secret := newDeploymentControllerTestSecret()
		secretProjected := newDeploymentControllerTestSecretProjected()

		mocks.initializeCanary(t)

		depPrimary, err := mocks.kubeClient.AppsV1().Deployments("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, "podinfo-secret-vol-primary",
				depPrimary.Spec.Template.Spec.Volumes[1].VolumeSource.Secret.SecretName)
		}

		secretPrimaryInit, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-init-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryInit.Data["apiKey"]))
			assert.Equal(t, string(secret.StringData["apiKey_string"]), string(secretPrimaryInit.StringData["apiKey_string"]))
		}

		secretPrimaryInitEnv, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-init-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryInitEnv.Data["apiKey"]))
			assert.Equal(t, string(secret.StringData["apiKey_string"]), string(secretPrimaryInitEnv.StringData["apiKey_string"]))
		}

		secretPrimary, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimary.Data["apiKey"]))
			assert.Equal(t, string(secret.StringData["apiKey_string"]), string(secretPrimary.StringData["apiKey_string"]))
		}

		secretPrimaryEnv, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryEnv.Data["apiKey"]))
			assert.Equal(t, string(secret.StringData["apiKey_string"]), string(secretPrimaryEnv.StringData["apiKey_string"]))
		}

		secretPrimaryVol, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-vol-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryVol.Data["apiKey"]))
			assert.Equal(t, string(secret.StringData["apiKey_string"]), string(secretPrimaryVol.StringData["apiKey_string"]))
		}

		secretProjectedName := depPrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[1].Secret.Name
		assert.Equal(t, "podinfo-secret-projected-primary", secretProjectedName)

		secretPrimaryProjected, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-projected-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secretProjected.Data["apiKey"]), string(secretPrimaryProjected.Data["apiKey"]))
		}

		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-enabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-enabled-primary", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-disabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-disabled-primary", metav1.GetOptions{})
		assert.Error(t, err)

		var trackedVolPresent, originalVolPresent bool
		for _, vol := range depPrimary.Spec.Template.Spec.Volumes {
			if vol.Secret != nil {
				switch vol.Secret.SecretName {
				case "podinfo-secret-tracker-enabled":
					assert.Fail(t, "primary Deployment does not contain a volume for config-tracked secret %q", vol.Secret.SecretName)
				case "podinfo-secret-tracker-enabled-primary":
					trackedVolPresent = true
				case "podinfo-secret-tracker-disabled":
					originalVolPresent = true
				case "podinfo-secret-tracker-disabled-primary":
					assert.Fail(t, "primary Deployment incorrectly contains a volume for a copy of an untracked secret %q", vol.Secret.SecretName)
				}
			}
		}
		assert.True(t, trackedVolPresent, "Volume for primary copy of config-tracked secret should be present")
		assert.True(t, originalVolPresent, "Volume for original secret with disabled tracking should be present")
	})

	t.Run("daemonset", func(t *testing.T) {
		dc := daemonsetConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDaemonSetFixture(dc)
		secret := newDaemonSetControllerTestSecret()
		secretProjected := newDaemonSetControllerTestSecretProjected()

		mocks.controller.Initialize(mocks.canary)

		daePrimary, err := mocks.kubeClient.AppsV1().DaemonSets("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, "podinfo-secret-vol-primary",
				daePrimary.Spec.Template.Spec.Volumes[1].VolumeSource.Secret.SecretName)
		}

		secretPrimaryInit, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-init-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryInit.Data["apiKey"]))
		}

		secretPrimaryInitEnv, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-init-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryInitEnv.Data["apiKey"]))
		}

		secretPrimary, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimary.Data["apiKey"]))
		}

		secretPrimaryEnv, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-all-env-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryEnv.Data["apiKey"]))
		}

		secretPrimaryVol, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-vol-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secret.Data["apiKey"]), string(secretPrimaryVol.Data["apiKey"]))
		}

		secretProjectedName := daePrimary.Spec.Template.Spec.Volumes[2].VolumeSource.Projected.Sources[1].Secret.Name
		assert.Equal(t, "podinfo-secret-projected-primary", secretProjectedName)

		secretPrimaryProjected, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-projected-primary", metav1.GetOptions{})
		if assert.NoError(t, err) {
			assert.Equal(t, string(secretProjected.Data["apiKey"]), string(secretPrimaryProjected.Data["apiKey"]))
		}

		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-enabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-enabled-primary", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-disabled", metav1.GetOptions{})
		assert.NoError(t, err)
		_, err = mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-tracker-disabled-primary", metav1.GetOptions{})
		assert.Error(t, err)

		var trackedVolPresent, originalVolPresent bool
		for _, vol := range daePrimary.Spec.Template.Spec.Volumes {
			if vol.Secret != nil {
				switch vol.Secret.SecretName {
				case "podinfo-secret-tracker-enabled":
					assert.Fail(t, "primary Deployment does not contain a volume for config-tracked secret %q", vol.Secret.SecretName)
				case "podinfo-secret-tracker-enabled-primary":
					trackedVolPresent = true
				case "podinfo-secret-tracker-disabled":
					originalVolPresent = true
				case "podinfo-secret-tracker-disabled-primary":
					assert.Fail(t, "primary Deployment incorrectly contains a volume for a copy of an untracked secret %q", vol.Secret.SecretName)
				}
			}
		}
		assert.True(t, trackedVolPresent, "Volume for primary copy of config-tracked secret should be present")
		assert.True(t, originalVolPresent, "Volume for original secret with disabled tracking should be present")
	})
}

func TestConfigTracker_HasConfigChanged_ShouldReturnErrorWhenAPIServerIsDown(t *testing.T) {
	t.Run("secret", func(t *testing.T) {
		dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks, kubeClient := newCustomizableFixture(dc)

		kubeClient.PrependReactor("get", "secrets", func(action k8sTesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("server error")
		})

		_, err := mocks.controller.configTracker.HasConfigChanged(mocks.canary)
		assert.Error(t, err)
	})

	t.Run("configmap", func(t *testing.T) {
		dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks, kubeClient := newCustomizableFixture(dc)

		kubeClient.PrependReactor("get", "configmaps", func(action k8sTesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("server error")
		})

		_, err := mocks.controller.configTracker.HasConfigChanged(mocks.canary)
		assert.Error(t, err)
	})
}

func Test_fieldIsMandatory(t *testing.T) {
	falsy := false
	truthy := true
	tests := []struct {
		optional *bool
		expected bool
	}{
		{
			optional: nil,
			expected: true,
		},
		{
			optional: &falsy,
			expected: true,
		},
		{
			optional: &truthy,
			expected: false,
		},
	}

	for _, tt := range tests {
		actual := fieldIsMandatory(tt.optional)
		assert.Equal(t, tt.expected, actual)
	}
}

func TestConfigTracker_ConfigOwnerMultiDeployment(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		dc := deploymentConfigs{name: "podinfo", label: "name", labelValue: "podinfo"}
		mocks := newDeploymentFixture(dc)
		mocks.initializeCanary(t)

		dep := newDeploymentControllerTest(dc)
		dep.Name = "podinfo2"
		canary := newDeploymentControllerTestCanary(canaryConfigs{targetName: "podinfo2"})
		canary.Name = "podinfo2"
		canary.Spec.TargetRef.Name = dep.Name
		mocks.kubeClient.AppsV1().Deployments("default").Create(context.TODO(), dep, metav1.CreateOptions{})
		mocks.controller.Initialize(canary)

		configMapPrimary, err := mocks.kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "podinfo-config-env-primary", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, configMapPrimary.OwnerReferences, 2)

		secretPrimary, err := mocks.kubeClient.CoreV1().Secrets("default").Get(context.TODO(), "podinfo-secret-env-primary", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Len(t, secretPrimary.OwnerReferences, 2)
	})
}
