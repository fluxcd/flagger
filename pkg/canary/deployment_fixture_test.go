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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
	"github.com/fluxcd/flagger/pkg/logger"
)

type deploymentControllerFixture struct {
	canary        *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	controller    DeploymentController
	logger        *zap.SugaredLogger
}

type canaryConfigs struct {
	targetName string
}

type deploymentConfigs struct {
	name       string
	labelValue string
	label      string
}

func (d deploymentControllerFixture) initializeCanary(t *testing.T) {
	_, err := d.controller.Initialize(d.canary)
	require.Error(t, err) // not ready yet

	primaryName := fmt.Sprintf("%s-primary", d.canary.Spec.TargetRef.Name)
	p, err := d.controller.kubeClient.AppsV1().
		Deployments(d.canary.Namespace).Get(context.TODO(), primaryName, metav1.GetOptions{})
	require.NoError(t, err)

	p.Status = appsv1.DeploymentStatus{
		Replicas:          1,
		UpdatedReplicas:   1,
		ReadyReplicas:     1,
		AvailableReplicas: 1,
	}

	_, err = d.controller.kubeClient.AppsV1().Deployments(d.canary.Namespace).Update(context.TODO(), p, metav1.UpdateOptions{})
	require.NoError(t, err)

	_, err = d.controller.Initialize(d.canary)
	require.NoError(t, err)
}

func newDeploymentFixture(dc deploymentConfigs) deploymentControllerFixture {
	fixture, _ := newCustomizableFixture(dc)
	return fixture
}

func newCustomizableFixture(dc deploymentConfigs) (deploymentControllerFixture, *fake.Clientset) {
	// init canary
	cc := canaryConfigs{targetName: dc.name}
	canary := newDeploymentControllerTestCanary(cc)
	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	// init kube clientset and register mock objects
	kubeClient := fake.NewSimpleClientset(
		newDeploymentControllerTest(dc),
		newDeploymentControllerTestConfigMap(),
		newDeploymentControllerTestConfigMapEnv(),
		newDeploymentControllerTestConfigMapVol(),
		newDeploymentControllerTestConfigProjected(),
		newDeploymentControllerTestConfigMapTrackerEnabled(),
		newDeploymentControllerTestConfigMapTrackerDisabled(),
		newDeploymentControllerTestConfigMapInit(),
		newDeploymentControllerTestConfigMapInitEnv(),
		newDeploymentControllerTestSecret(),
		newDeploymentControllerTestSecretEnv(),
		newDeploymentControllerTestSecretVol(),
		newDeploymentControllerTestSecretProjected(),
		newDeploymentControllerTestSecretTrackerEnabled(),
		newDeploymentControllerTestSecretTrackerDisabled(),
		newDeploymentControllerTestSecretInit(),
		newDeploymentControllerTestSecretInitEnv(),
	)

	logger, _ := logger.NewLogger("debug")

	ctrl := DeploymentController{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
		labels:        []string{"app", "name"},
		configTracker: &ConfigTracker{
			Logger:        logger,
			KubeClient:    kubeClient,
			FlaggerClient: flaggerClient,
		},
		includeLabelPrefix: []string{"app.kubernetes.io"},
	}

	return deploymentControllerFixture{
		canary:        canary,
		controller:    ctrl,
		logger:        logger,
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
	}, kubeClient
}

func newDeploymentControllerTestConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-env",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigMapV2() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-env",
		},
		Data: map[string]string{
			"color":  "blue",
			"output": "console",
		},
	}
}

func newDeploymentControllerTestConfigMapInit() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-init-env",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigMapInitEnv() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-init-all-env",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigProjected() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-projected",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigMapEnv() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-all-env",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigMapTrackerEnabled() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-tracker-enabled",
			Annotations: map[string]string{
				"unrelated-annotation-1":      ":)",
				"flagger.app/config-tracking": "enabled",
				"unrelated-annotation-2":      "<3",
			},
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigMapTrackerDisabled() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-tracker-disabled",
			Annotations: map[string]string{
				"unrelated-annotation-1":      "c:",
				"flagger.app/config-tracking": "disabled",
				"unrelated-annotation-2":      "^-^",
			},
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestConfigMapVol() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-vol",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
}

func newDeploymentControllerTestSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretProjected() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-projected",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretEnv() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-all-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretVol() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-vol",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretTrackerEnabled() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-tracker-enabled",
			Annotations: map[string]string{
				"unrelated-annotation-1":      ":)",
				"flagger.app/config-tracking": "enabled",
				"unrelated-annotation-2":      "<3",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretTrackerDisabled() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-tracker-disabled",
			Annotations: map[string]string{
				"unrelated-annotation-1":      "c:",
				"flagger.app/config-tracking": "disabled",
				"unrelated-annotation-2":      "^-^",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretInit() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-init-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestSecretInitEnv() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-init-all-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
}

func newDeploymentControllerTestCanary(cc canaryConfigs) *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
				Name:       cc.targetName,
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			AutoscalerRef: &flaggerv1.AutoscalerRefernce{
				Name:       "podinfo",
				APIVersion: "autoscaling/v2beta2",
				Kind:       "HorizontalPodAutoscaler",
			}, Service: flaggerv1.CanaryService{
				Port: 9898,
			}, Analysis: &flaggerv1.CanaryAnalysis{
				Threshold:  10,
				StepWeight: 10,
				MaxWeight:  50,
			},
		},
	}
	return cd
}

func newDeploymentControllerTest(dc deploymentConfigs) *appsv1.Deployment {
	optional := false
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      dc.name,
			Annotations: map[string]string{
				"kustomize.toolkit.fluxcd.io/checksum": "0a40893bfdc545d62125bd3e74eeb2ebaa7097c2",
				"test-annotation-1":                    "test-annotation-value-1",
			},
			Labels: map[string]string{
				"test-label-1": "test-label-value-1",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					dc.label: dc.labelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"test-annotation-1": "test-annotation-value-1",
					},
					Labels: map[string]string{
						dc.label:       dc.labelValue,
						"test-label-1": "test-label-value-1",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name: "PODINFO_UI_COLOR",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "podinfo-config-init-env",
											},
											Key: "color",
										},
									},
								},
								{
									Name: "API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "podinfo-secret-init-env",
											},
											Key: "apiKey",
										},
									},
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "podinfo-config-init-all-env",
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "podinfo-secret-init-all-env",
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "podinfo",
							Image: "quay.io/stefanprodan/podinfo:1.2.0",
							Command: []string{
								"./podinfo",
								"--port=9898",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: *resource.NewQuantity(1000, resource.DecimalExponent),
								},
							},
							Args:       nil,
							WorkingDir: "",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "PODINFO_UI_COLOR",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "podinfo-config-env",
											},
											Key: "color",
										},
									},
								},
								{
									Name: "API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "podinfo-secret-env",
											},
											Key: "apiKey",
										},
									},
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "podinfo-config-all-env",
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "podinfo-secret-all-env",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/podinfo/config",
									ReadOnly:  true,
								},
								{
									Name:      "secret",
									MountPath: "/etc/podinfo/secret",
									ReadOnly:  true,
								},
								{
									Name:      "config-tracker-enabled",
									MountPath: "/etc/podinfo/config-tracker-enabled",
									ReadOnly:  true,
								},
								{
									Name:      "config-tracker-disabled",
									MountPath: "/etc/podinfo/config-tracker-disabled",
									ReadOnly:  true,
								},
								{
									Name:      "secret-tracker-enabled",
									MountPath: "/etc/podinfo/secret-tracker-enabled",
									ReadOnly:  true,
								},
								{
									Name:      "secret-tracker-disabled",
									MountPath: "/etc/podinfo/secret-tracker-disabled",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-vol",
									},
									Optional: &optional,
								},
							},
						},
						{
							Name: "secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-vol",
									Optional:   &optional,
								},
							},
						},
						{
							Name: "projected",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ConfigMap: &corev1.ConfigMapProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "podinfo-config-projected",
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "color",
														Path: "my-group/my-color",
													},
												},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "podinfo-secret-projected",
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "apiKey",
														Path: "my-group/my-api-key",
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Name: "config-tracker-enabled",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-tracker-enabled",
									},
								},
							},
						},
						{
							Name: "config-tracker-disabled",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-tracker-disabled",
									},
								},
							},
						},
						{
							Name: "secret-tracker-enabled",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-tracker-enabled",
								},
							},
						},
						{
							Name: "secret-tracker-disabled",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-tracker-disabled",
								},
							},
						},
					},
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:    "app",
													Values: []string{"podinfo"},
												},
											},
										},
									},
								},
								{
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:    "app",
													Values: []string{"arbitrary-app"},
												},
											},
										},
									},
								},
							},
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:    "app",
												Values: []string{"podinfo"},
											},
										},
									},
								},
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:    "app",
												Values: []string{"arbitrary-app"},
											},
										},
									},
								},
							},
						},
					},
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:    "app",
										Values: []string{"podinfo"},
									},
								},
							},
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:    "app",
										Values: []string{"arbitrary-app"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return d
}

func newDeploymentControllerTestV2() *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
			Annotations: map[string]string{
				"test-annotation-1": "test-annotation-value-1",
			},
			Labels: map[string]string{
				"test-label-1": "test-label-value-1",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "podinfo",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name":         "podinfo",
						"test-label-1": "test-label-value-1",
					},
					Annotations: map[string]string{
						"test-annotation-1": "test-annotation-value-1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "podinfo",
							Image: "quay.io/stefanprodan/podinfo:1.2.1",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Command: []string{
								"./podinfo",
								"--port=9898",
							},
							Env: []corev1.EnvVar{
								{
									Name: "PODINFO_UI_COLOR",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "podinfo-config-env",
											},
											Key: "color",
										},
									},
								},
								{
									Name: "API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "podinfo-secret-env",
											},
											Key: "apiKey",
										},
									},
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "podinfo-config-all-env",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/podinfo/config",
									ReadOnly:  true,
								},
								{
									Name:      "secret",
									MountPath: "/etc/podinfo/secret",
									ReadOnly:  true,
								},
								{
									Name:      "config-tracker-enabled",
									MountPath: "/etc/podinfo/config-tracker-enabled",
									ReadOnly:  true,
								},
								{
									Name:      "config-tracker-disabled",
									MountPath: "/etc/podinfo/config-tracker-disabled",
									ReadOnly:  true,
								},
								{
									Name:      "secret-tracker-enabled",
									MountPath: "/etc/podinfo/secret-tracker-enabled",
									ReadOnly:  true,
								},
								{
									Name:      "secret-tracker-disabled",
									MountPath: "/etc/podinfo/secret-tracker-disabled",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-vol",
									},
								},
							},
						},
						{
							Name: "secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-vol",
								},
							},
						},
						{
							Name: "projected",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ConfigMap: &corev1.ConfigMapProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "podinfo-config-projected",
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "color",
														Path: "my-group/my-color",
													},
												},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "podinfo-secret-projected",
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "apiKey",
														Path: "my-group/my-api-key",
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Name: "config-tracker-enabled",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-tracker-enabled",
									},
								},
							},
						},
						{
							Name: "config-tracker-disabled",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-tracker-disabled",
									},
								},
							},
						},
						{
							Name: "secret-tracker-enabled",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-tracker-enabled",
								},
							},
						},
						{
							Name: "secret-tracker-disabled",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-tracker-disabled",
								},
							},
						},
					},
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:    "app",
													Values: []string{"podinfo"},
												},
											},
										},
									},
								},
							},
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:    "app",
												Values: []string{"podinfo"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return d
}
