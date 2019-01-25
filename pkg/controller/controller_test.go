package controller

import (
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	fakeIstio "github.com/knative/pkg/client/clientset/versioned/fake"
	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/stefanprodan/flagger/pkg/client/clientset/versioned/fake"
	informers "github.com/stefanprodan/flagger/pkg/client/informers/externalversions"
	"github.com/stefanprodan/flagger/pkg/logging"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v1"
	hpav2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sync"
	"time"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

func SetupTest() (
	canary *v1alpha3.Canary,
	kubeClient kubernetes.Interface,
	istioClient istioclientset.Interface,
	flaggerClient clientset.Interface,
	deployer CanaryDeployer,
	router CanaryRouter,
	observer CanaryObserver,
	controller *Controller,
	logger *zap.SugaredLogger,
) {
	canary = newTestCanary()
	configMap := NewTestConfigMap()
	configMapForEnv := NewTestConfigMapForEnv()
	secret := NewTestSecret()
	dep := newTestDeployment()
	hpa := newTestHPA()

	kubeClient = fake.NewSimpleClientset(secret, configMap, configMapForEnv, dep, hpa)

	istioClient = fakeIstio.NewSimpleClientset()

	flaggerClient = fakeFlagger.NewSimpleClientset(canary)

	logger, _ = logging.NewLogger("debug")

	observer = CanaryObserver{
		metricsServer: "fake",
	}

	deployer = CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
		configTracker: ConfigTracker{
			logger:        logger,
			kubeClient:    kubeClient,
			flaggerClient: flaggerClient,
		},
	}

	router = CanaryRouter{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		logger:        logger,
	}

	controller = newTestController(kubeClient, istioClient, flaggerClient, logger, deployer, router, observer)

	return
}

func newTestController(
	kubeClient kubernetes.Interface,
	istioClient istioclientset.Interface,
	flaggerClient clientset.Interface,
	logger *zap.SugaredLogger,
	deployer CanaryDeployer,
	router CanaryRouter,
	observer CanaryObserver,
) *Controller {
	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, noResyncPeriodFunc())
	flaggerInformer := flaggerInformerFactory.Flagger().V1alpha3().Canaries()

	ctrl := &Controller{
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		flaggerClient: flaggerClient,
		flaggerLister: flaggerInformer.Lister(),
		flaggerSynced: flaggerInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		eventRecorder: &record.FakeRecorder{},
		logger:        logger,
		canaries:      new(sync.Map),
		flaggerWindow: time.Second,
		deployer:      deployer,
		router:        router,
		observer:      observer,
		recorder:      NewCanaryRecorder(false),
	}
	ctrl.flaggerSynced = alwaysReady

	return ctrl
}

func NewTestConfigMap() *corev1.ConfigMap {
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

func NewTestConfigMapUpdated() *corev1.ConfigMap {
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

func NewTestConfigMapForEnv() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-all-env",
		},
		Data: map[string]string{
			"a": "b",
			"c": "d",
		},
	}
}

func NewTestConfigMapForEnvUpdated() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-config-all-env",
		},
		Data: map[string]string{
			"x": "y",
			"c": "d",
		},
	}
}

func NewTestSecret() *corev1.Secret {
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

func NewTestSecretUpdated() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo-secret-env",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test2"),
		},
	}
}

func newTestCanary() *v1alpha3.Canary {
	cd := &v1alpha3.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha3.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: v1alpha3.CanarySpec{
			TargetRef: hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			AutoscalerRef: &hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "autoscaling/v2beta1",
				Kind:       "HorizontalPodAutoscaler",
			}, Service: v1alpha3.CanaryService{
				Port: 9898,
			}, CanaryAnalysis: v1alpha3.CanaryAnalysis{
				Threshold:  10,
				StepWeight: 10,
				MaxWeight:  50,
				Metrics: []v1alpha3.CanaryMetric{
					{
						Name:      "istio_requests_total",
						Threshold: 99,
						Interval:  "1m",
					},
					{
						Name:      "istio_request_duration_seconds_bucket",
						Threshold: 500,
						Interval:  "1m",
					},
				},
			},
		},
	}
	return cd
}

func newTestDeployment() *appsv1.Deployment {
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
						},
					},
				},
			},
		},
	}

	return d
}

func newTestDeploymentUpdated() *appsv1.Deployment {
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
						},
					},
				},
			},
		},
	}

	return d
}

func newTestHPA() *hpav2.HorizontalPodAutoscaler {
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
