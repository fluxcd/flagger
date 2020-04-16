package controller

import (
	"sync"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
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

type daemonSetFixture struct {
	canary        *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	meshClient    clientset.Interface
	flaggerClient clientset.Interface
	deployer      canary.Controller
	ctrl          *Controller
	logger        *zap.SugaredLogger
	router        router.Interface
}

func newDaemonSetFixture(c *flaggerv1.Canary) daemonSetFixture {
	if c == nil {
		c = newDaemonSetTestCanary()
	}

	// init Flagger clientset and register objects
	flaggerClient := fakeFlagger.NewSimpleClientset(
		c,
		newDaemonSetTestMetricTemplate(),
		newDaemonSetTestAlertProvider(),
	)

	// init Kubernetes clientset and register objects
	kubeClient := fake.NewSimpleClientset(
		newDaemonSetTestDaemonSet(),
		newDaemonSetTestService(),
		newDaemonSetTestConfigMap(),
		newDaemonSetTestConfigMapEnv(),
		newDaemonSetTestConfigMapVol(),
		newDaemonSetTestSecret(),
		newDaemonSetTestSecretEnv(),
		newDaemonSetTestSecretVol(),
		newDaemonSetTestAlertProviderSecret(),
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
	ctrl.flaggerInformers.MetricInformer.Informer().GetIndexer().Add(newDaemonSetTestMetricTemplate())
	ctrl.flaggerInformers.AlertInformer.Informer().GetIndexer().Add(newDaemonSetTestAlertProvider())

	meshRouter := rf.MeshRouter("istio")

	return daemonSetFixture{
		canary:        c,
		deployer:      canaryFactory.Controller("DaemonSet"),
		logger:        logger,
		flaggerClient: flaggerClient,
		meshClient:    flaggerClient,
		kubeClient:    kubeClient,
		ctrl:          ctrl,
		router:        meshRouter,
	}
}

func newDaemonSetTestConfigMap() *corev1.ConfigMap {
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

func newDaemonSetTestConfigMapV2() *corev1.ConfigMap {
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

func newDaemonSetTestConfigMapEnv() *corev1.ConfigMap {
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

func newDaemonSetTestConfigMapVol() *corev1.ConfigMap {
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

func newDaemonSetTestSecret() *corev1.Secret {
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

func newDaemonSetTestSecretV2() *corev1.Secret {
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

func newDaemonSetTestSecretEnv() *corev1.Secret {
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

func newDaemonSetTestSecretVol() *corev1.Secret {
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

func newDaemonSetTestCanary() *flaggerv1.Canary {
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
				Kind:       "DaemonSet",
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

func newDaemonSetTestCanaryMirror() *flaggerv1.Canary {
	cd := newDaemonSetTestCanary()
	cd.Spec.Analysis.Mirror = true
	return cd
}

func newDaemonSetTestCanaryAB() *flaggerv1.Canary {
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
				Kind:       "DaemonSet",
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

func newDaemonSetTestDaemonSet() *appsv1.DaemonSet {
	d := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: appsv1.DaemonSetSpec{
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

func newDaemonSetTestDaemonSetV2() *appsv1.DaemonSet {
	d := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: appsv1.DaemonSetSpec{
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

func newDaemonSetTestService() *corev1.Service {
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

func newDaemonSetTestMetricTemplate() *flaggerv1.MetricTemplate {
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

func newDaemonSetTestAlertProviderSecret() *corev1.Secret {
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

func newDaemonSetTestAlertProvider() *flaggerv1.AlertProvider {
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
