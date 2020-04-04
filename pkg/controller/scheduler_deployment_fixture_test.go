package controller

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha1 "github.com/weaveworks/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
	"github.com/weaveworks/flagger/pkg/canary"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
	informers "github.com/weaveworks/flagger/pkg/client/informers/externalversions"
	"github.com/weaveworks/flagger/pkg/logger"
	"github.com/weaveworks/flagger/pkg/metrics"
	"github.com/weaveworks/flagger/pkg/metrics/observers"
	"github.com/weaveworks/flagger/pkg/notifier"
	"github.com/weaveworks/flagger/pkg/router"
)

type fixture struct {
	canary        *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	meshClient    clientset.Interface
	flaggerClient clientset.Interface
	deployer      canary.Controller
	ctrl          *Controller
	logger        *zap.SugaredLogger
	router        router.Interface
}

func (f fixture) makePrimaryReady(t *testing.T) {
	primaryName := fmt.Sprintf("%s-primary", f.canary.Spec.TargetRef.Name)
	f.makeReady(t, primaryName)
}

func (f fixture) makeCanaryReady(t *testing.T) {
	f.makeReady(t, f.canary.Spec.TargetRef.Name)
}

func (f fixture) makeReady(t *testing.T, name string) {
	p, err := f.kubeClient.AppsV1().
		Deployments("default").
		Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(t, err)

	p.Status = appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1,
		ReadyReplicas: 1, AvailableReplicas: 1}

	_, err = f.kubeClient.AppsV1().Deployments("default").Update(context.TODO(), p, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func newDeploymentFixture(c *flaggerv1.Canary) fixture {
	if c == nil {
		c = newDeploymentTestCanary()
	}

	// init Flagger clientset and register objects
	flaggerClient := fakeFlagger.NewSimpleClientset(
		c,
		newDeploymentTestMetricTemplate(),
		newDeploymentTestAlertProvider(),
	)

	// init Kubernetes clientset and register objects
	kubeClient := fake.NewSimpleClientset(
		newDeploymentTestDeployment(),
		newDeploymentTestService(),
		newDeploymentTestHPA(),
		newDeploymentTestConfigMap(),
		newDeploymentTestConfigMapEnv(),
		newDeploymentTestConfigMapVol(),
		newDeploymentTestSecret(),
		newDeploymentTestSecretEnv(),
		newDeploymentTestSecretVol(),
		newDeploymentTestAlertProviderSecret(),
	)

	logger, _ := logger.NewLogger("debug")

	// init controller
	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, 0)

	fi := Informers{
		CanaryInformer: flaggerInformerFactory.Flagger().V1beta1().Canaries(),
		MetricInformer: flaggerInformerFactory.Flagger().V1beta1().MetricTemplates(),
		AlertInformer:  flaggerInformerFactory.Flagger().V1beta1().AlertProviders(),
	}

	// init router
	rf := router.NewFactory(nil, kubeClient, flaggerClient, "annotationsPrefix", logger, flaggerClient)

	// init observer
	observerFactory, _ := observers.NewFactory("fake")

	// init canary factory
	configTracker := &canary.ConfigTracker{
		Logger:        logger,
		KubeClient:    kubeClient,
		FlaggerClient: flaggerClient,
	}
	canaryFactory := canary.NewFactory(kubeClient, flaggerClient, configTracker, []string{"app", "name"}, logger)

	ctrl := &Controller{
		kubeClient:       kubeClient,
		flaggerClient:    flaggerClient,
		flaggerInformers: fi,
		flaggerSynced:    fi.CanaryInformer.Informer().HasSynced,
		workqueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		eventRecorder:    &record.FakeRecorder{},
		logger:           logger,
		canaries:         new(sync.Map),
		flaggerWindow:    time.Second,
		canaryFactory:    canaryFactory,
		observerFactory:  observerFactory,
		recorder:         metrics.NewRecorder(controllerAgentName, false),
		routerFactory:    rf,
		notifier:         &notifier.NopNotifier{},
	}
	ctrl.flaggerSynced = alwaysReady
	ctrl.flaggerInformers.CanaryInformer.Informer().GetIndexer().Add(c)
	ctrl.flaggerInformers.MetricInformer.Informer().GetIndexer().Add(newDeploymentTestMetricTemplate())
	ctrl.flaggerInformers.AlertInformer.Informer().GetIndexer().Add(newDeploymentTestAlertProvider())

	meshRouter := rf.MeshRouter("istio")

	return fixture{
		canary:        c,
		deployer:      canaryFactory.Controller("Deployment"),
		logger:        logger,
		flaggerClient: flaggerClient,
		meshClient:    flaggerClient,
		kubeClient:    kubeClient,
		ctrl:          ctrl,
		router:        meshRouter,
	}
}

func newDeploymentTestConfigMap() *corev1.ConfigMap {
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

func newDeploymentTestConfigMapV2() *corev1.ConfigMap {
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

func newDeploymentTestConfigMapEnv() *corev1.ConfigMap {
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

func newDeploymentTestConfigMapVol() *corev1.ConfigMap {
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

func newDeploymentTestSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey":   []byte("test"),
			"username": []byte("test"),
			"password": []byte("test"),
		},
	}
}

func newDeploymentTestSecretV2() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey":   []byte("test2"),
			"username": []byte("test"),
			"password": []byte("test"),
		},
	}
}

func newDeploymentTestSecretEnv() *corev1.Secret {
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

func newDeploymentTestSecretVol() *corev1.Secret {
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

func newDeploymentTestCanary() *flaggerv1.Canary {
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
				Metrics: []flaggerv1.CanaryMetric{
					{
						Name:      "request-success-rate",
						Threshold: 99,
						Interval:  "1m",
					},
					{
						Name: "request-duration",
						ThresholdRange: &flaggerv1.CanaryThresholdRange{
							Min: toFloatPtr(0),
							Max: toFloatPtr(500000),
						},
						Interval: "1m",
					},
					{
						Name: "custom",
						ThresholdRange: &flaggerv1.CanaryThresholdRange{
							Min: toFloatPtr(0),
							Max: toFloatPtr(100),
						},
						Interval: "1m",
						TemplateRef: &flaggerv1.CrossNamespaceObjectReference{
							Name:      "envoy",
							Namespace: "default",
						},
					},
				},
			},
		},
	}
	return cd
}

func newDeploymentTestCanaryMirror() *flaggerv1.Canary {
	cd := newDeploymentTestCanary()
	cd.Spec.Analysis.Mirror = true
	return cd
}

func newDeploymentTestCanaryAB() *flaggerv1.Canary {
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
				Iterations: 10,
				Match: []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"x-user-type": {
								Exact: "test",
							},
						},
					},
				},
				Metrics: []flaggerv1.CanaryMetric{
					{
						Name: "request-success-rate",
						ThresholdRange: &flaggerv1.CanaryThresholdRange{
							Min: toFloatPtr(99),
							Max: toFloatPtr(100),
						},
						Interval: "1m",
					},
					{
						Name:      "request-duration",
						Threshold: 500000,
						Interval:  "1m",
					},
					{
						Name: "custom",
						ThresholdRange: &flaggerv1.CanaryThresholdRange{
							Min: toFloatPtr(0),
							Max: toFloatPtr(500000),
						},
						Interval: "1m",
						Query:    "fake",
					},
				},
			},
		},
	}
	return cd
}

func newDeploymentTestDeployment() *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "podinfo",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "podinfo",
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
							Args:       nil,
							WorkingDir: "",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "http-metrics",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: 8888,
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
					},
				},
			},
		},
	}

	return d
}

func newDeploymentTestDeploymentV2() *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "podinfo",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "podinfo",
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
					},
				},
			},
		},
	}

	return d
}

func newDeploymentTestService() *corev1.Service {
	d := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "podinfo",
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       9898,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}

	return d
}

func newDeploymentTestServiceV2() *corev1.Service {
	d := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "podinfo-v2",
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       9898,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}

	return d
}

func newDeploymentTestHPA() *hpav2.HorizontalPodAutoscaler {
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

func newDeploymentTestMetricTemplate() *flaggerv1.MetricTemplate {
	provider := flaggerv1.MetricTemplateProvider{
		Type:    "prometheus",
		Address: "fake",
		SecretRef: &corev1.LocalObjectReference{
			Name: "podinfo-secret-env",
		},
	}

	template := &flaggerv1.MetricTemplate{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "envoy",
		},
		Spec: flaggerv1.MetricTemplateSpec{
			Provider: provider,
			Query:    `sum(envoy_cluster_upstream_rq{envoy_cluster_name=~"{{ namespace }}_{{ target }}"})`,
		},
	}
	return template
}

func newDeploymentTestAlertProviderSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "alert-secret",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"address": []byte("http://mock.slack"),
		},
	}
}

func newDeploymentTestAlertProvider() *flaggerv1.AlertProvider {
	return &flaggerv1.AlertProvider{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "slack",
		},
		Spec: flaggerv1.AlertProviderSpec{
			Type:    "slack",
			Address: "http://fake.slack",
			SecretRef: &corev1.LocalObjectReference{
				Name: "alert-secret",
			},
		},
	}
}
