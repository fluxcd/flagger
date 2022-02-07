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

type daemonSetControllerFixture struct {
	canary        *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	controller    DaemonSetController
	logger        *zap.SugaredLogger
}

type daemonsetConfigs struct {
	name       string
	labelValue string
	label      string
}

func newDaemonSetFixture(dc daemonsetConfigs) daemonSetControllerFixture {
	// init canary
	canary := newDaemonSetControllerTestCanary(dc)
	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	// init kube clientset and register mock objects
	kubeClient := fake.NewSimpleClientset(
		newDaemonSetControllerTestPodInfo(dc),
		newDaemonSetControllerTestConfigMap(),
		newDaemonSetControllerTestConfigMapEnv(),
		newDaemonSetControllerTestConfigMapVol(),
		newDaemonSetControllerTestConfigProjected(),
		newDaemonSetControllerTestConfigMapTrackerEnabled(),
		newDaemonSetControllerTestConfigMapTrackerDisabled(),
		newDaemonSetControllerTestConfigMapInit(),
		newDaemonSetControllerTestConfigMapInitEnv(),
		newDaemonSetControllerTestSecret(),
		newDaemonSetControllerTestSecretEnv(),
		newDaemonSetControllerTestSecretVol(),
		newDaemonSetControllerTestSecretProjected(),
		newDaemonSetControllerTestSecretTrackerEnabled(),
		newDaemonSetControllerTestSecretTrackerDisabled(),
		newDaemonSetControllerTestSecretInit(),
		newDaemonSetControllerTestSecretInitEnv(),
	)

	logger, _ := logger.NewLogger("debug")

	ctrl := DaemonSetController{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
		labels:        []string{"app", "name"},
		configTracker: &ConfigTracker{
			Logger:        logger,
			KubeClient:    kubeClient,
			FlaggerClient: flaggerClient,
		},
	}

	return daemonSetControllerFixture{
		canary:        canary,
		controller:    ctrl,
		logger:        logger,
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
	}
}

func newDaemonSetControllerTestConfigMap() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapV2() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigProjected() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapEnv() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapInit() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapInitEnv() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapVol() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapTrackerEnabled() *corev1.ConfigMap {
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

func newDaemonSetControllerTestConfigMapTrackerDisabled() *corev1.ConfigMap {
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

func newDaemonSetControllerTestSecret() *corev1.Secret {
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

func newDaemonSetControllerTestSecretProjected() *corev1.Secret {
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

func newDaemonSetControllerTestSecretEnv() *corev1.Secret {
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

func newDaemonSetControllerTestSecretVol() *corev1.Secret {
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

func newDaemonSetControllerTestSecretTrackerEnabled() *corev1.Secret {
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

func newDaemonSetControllerTestSecretTrackerDisabled() *corev1.Secret {
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

func newDaemonSetControllerTestSecretInit() *corev1.Secret {
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

func newDaemonSetControllerTestSecretInitEnv() *corev1.Secret {
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

func newDaemonSetControllerTestCanary(dc daemonsetConfigs) *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       dc.name,
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
			},
			Analysis: &flaggerv1.CanaryAnalysis{},
		},
	}
	return cd
}

func newDaemonSetControllerTestPodInfo(dc daemonsetConfigs) *appsv1.DaemonSet {
	d := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      dc.name,
			Annotations: map[string]string{
				"test-annotation-1": "test-annotation-value-1",
			},
			Labels: map[string]string{
				"test-label-1": "test-label-value-1",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					dc.label: dc.labelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						dc.label:       dc.labelValue,
						"test-label-1": "test-label-value-1",
					},
					Annotations: map[string]string{
						"test-annotation-1": "test-annotation-value-1",
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
				},
			},
		},
	}
	return d
}

func newDaemonSetControllerTestPodInfoV2() *appsv1.DaemonSet {
	d := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "podinfo",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "podinfo",
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
				},
			},
		},
	}
	return d
}
