/*
Copyright 2026 The Flux authors

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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestConfigTracker_GetTargetConfigs_RequiredWinsOverOptional(t *testing.T) {
	optional := true
	// The same ConfigMap and Secret are mounted as required volumes and also
	// referenced as optional env vars. Neither object exists.
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "podinfo", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "shared-configmap"},
								},
							},
						},
						{
							Name: "secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: "shared-secret"},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "podinfo",
						Env: []corev1.EnvVar{
							{
								Name: "COLOR",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "shared-configmap"},
										Key:                  "color",
										Optional:             &optional,
									},
								},
							},
							{
								Name: "TOKEN",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "shared-secret"},
										Key:                  "token",
										Optional:             &optional,
									},
								},
							},
						},
					}},
				},
			},
		},
	}
	cd := &flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{Name: "podinfo", Namespace: "default"},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{Name: "podinfo", Kind: "Deployment"},
		},
	}

	t.Run("configmap", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset(dep)
		kubeClient.PrependReactor("get", "secrets", func(action k8sTesting.Action) (bool, runtime.Object, error) {
			return true, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "shared-secret"}, Type: corev1.SecretTypeOpaque}, nil
		})
		tracker := &ConfigTracker{KubeClient: kubeClient, Logger: zap.NewNop().Sugar()}

		_, err := tracker.GetTargetConfigs(cd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shared-configmap")
	})

	t.Run("secret", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset(dep)
		kubeClient.PrependReactor("get", "configmaps", func(action k8sTesting.Action) (bool, runtime.Object, error) {
			return true, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared-configmap"}}, nil
		})
		tracker := &ConfigTracker{KubeClient: kubeClient, Logger: zap.NewNop().Sugar()}

		_, err := tracker.GetTargetConfigs(cd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shared-secret")
	})
}
