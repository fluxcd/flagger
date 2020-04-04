package canary

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
	"github.com/weaveworks/flagger/pkg/logger"
)

type deploymentControllerFixture struct {
	canary        *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	controller    DeploymentController
	logger        *zap.SugaredLogger
}

func (d deploymentControllerFixture) initializeCanary(t *testing.T) {
	err := d.controller.Initialize(d.canary)
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

	require.NoError(t, d.controller.Initialize(d.canary))
}

func newDeploymentFixture() deploymentControllerFixture {
	// init canary
	canary := newDeploymentControllerTestCanary()
	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	// init kube clientset and register mock objects
	kubeClient := fake.NewSimpleClientset(
		newDeploymentControllerTest(),
		newDeploymentControllerTestHPA(),
		newDeploymentControllerTestConfigMap(),
		newDeploymentControllerTestConfigMapEnv(),
		newDeploymentControllerTestConfigMapVol(),
		newDeploymentControllerTestConfigProjected(),
		newDeploymentControllerTestSecret(),
		newDeploymentControllerTestSecretEnv(),
		newDeploymentControllerTestSecretVol(),
		newDeploymentControllerTestSecretProjected(),
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
	}

	return deploymentControllerFixture{
		canary:        canary,
		controller:    ctrl,
		logger:        logger,
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
	}
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

func newDeploymentControllerTestCanary() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			AutoscalerRef: &flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "autoscaling/v2beta1",
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

func newDeploymentControllerTest() *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
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
						"name": "podinfo",
					},
				},
				Spec: corev1.PodSpec{
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
					},
				},
			},
		},
	}

	return d
}

func newDeploymentControllerTestHPA() *hpav2.HorizontalPodAutoscaler {
	h := &hpav2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{APIVersion: hpav2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: hpav2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: hpav2.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Metrics: []hpav2.MetricSpec{
				{
					Type: "Resource",
					Resource: &hpav2.ResourceMetricSource{
						Name:                     "cpu",
						TargetAverageUtilization: int32p(99),
					},
				},
			},
		},
	}

	return h
}
