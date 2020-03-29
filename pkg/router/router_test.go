package router

import (
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha1 "github.com/weaveworks/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
	"github.com/weaveworks/flagger/pkg/logger"
)

type fixture struct {
	canary        *flaggerv1.Canary
	abtest        *flaggerv1.Canary
	appmeshCanary *flaggerv1.Canary
	ingressCanary *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	meshClient    clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func newFixture(c *flaggerv1.Canary) fixture {
	canary := newTestCanary()
	if c != nil {
		canary = c
	}
	abtest := newTestABTest()
	appmeshCanary := newTestCanaryAppMesh()
	ingressCanary := newTestCanaryIngress()

	flaggerClient := fakeFlagger.NewSimpleClientset(
		canary,
		abtest,
		appmeshCanary,
		ingressCanary,
	)

	kubeClient := fake.NewSimpleClientset(
		newTestDeployment(),
		newTestABTestDeployment(),
		newTestIngress(),
	)

	meshClient := fakeFlagger.NewSimpleClientset()

	logger, _ := logger.NewLogger("debug")
	return fixture{
		canary:        canary,
		abtest:        abtest,
		appmeshCanary: appmeshCanary,
		ingressCanary: ingressCanary,
		kubeClient:    kubeClient,
		meshClient:    meshClient,
		flaggerClient: flaggerClient,
		logger:        logger,
	}
}

func newTestCanary() *flaggerv1.Canary {
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
			Service: flaggerv1.CanaryService{
				Port:          9898,
				PortDiscovery: true,
				Headers: &istiov1alpha3.Headers{
					Request: &istiov1alpha3.HeaderOperations{
						Add: map[string]string{
							"x-envoy-upstream-rq-timeout-ms": "15000",
						},
						Remove: []string{"test"},
					},
					Response: &istiov1alpha3.HeaderOperations{
						Remove: []string{"token"},
					},
				},
				CorsPolicy: &istiov1alpha3.CorsPolicy{
					AllowMethods: []string{
						"GET",
						"POST",
					},
				},
				Match: []istiov1alpha3.HTTPMatchRequest{
					{Uri: &istiov1alpha1.StringMatch{
						Prefix: "/podinfo",
					}},
				},
				Retries: &istiov1alpha3.HTTPRetry{
					Attempts:      10,
					PerTryTimeout: "30s",
				},
				Gateways: []string{
					"public-gateway.istio",
					"mesh",
				},
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
						Name:      "request-duration",
						Threshold: 500,
						Interval:  "1m",
					},
				},
			},
		},
	}
	return cd
}

func newTestCanaryAppMesh() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "appmesh",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Service: flaggerv1.CanaryService{
				Port:     9898,
				MeshName: "global",
				Hosts:    []string{"*"},
				Backends: []string{"backend.default"},
				Timeout:  "25",
				Retries: &istiov1alpha3.HTTPRetry{
					Attempts:      5,
					PerTryTimeout: "gateway-error",
					RetryOn:       "5s",
				},
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
						Name:      "request-duration",
						Threshold: 500,
						Interval:  "1m",
					},
				},
			},
		},
	}
	return cd
}

func newTestSMICanary() *flaggerv1.Canary {
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
			Service: flaggerv1.CanaryService{
				Name:     "podinfo",
				Port:     80,
				PortName: "http",
				TargetPort: intstr.IntOrString{
					Type:   0,
					IntVal: 9898,
				},
				PortDiscovery: true,
			},
			Analysis: &flaggerv1.CanaryAnalysis{
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
						Name:      "request-duration",
						Threshold: 500,
						Interval:  "1m",
					},
				},
			},
		},
	}
	return cd
}

func newTestMirror() *flaggerv1.Canary {
	cd := newTestCanary()
	cd.GetAnalysis().Mirror = true
	return cd
}

func newTestABTest() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "abtest",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       "abtest",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Service: flaggerv1.CanaryService{
				Port:     9898,
				MeshName: "global",
			}, Analysis: &flaggerv1.CanaryAnalysis{
				Threshold:  10,
				Iterations: 2,
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
						Name:      "request-success-rate",
						Threshold: 99,
						Interval:  "1m",
					},
					{
						Name:      "request-duration",
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
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "9797",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "podinfo",
							Image: "stefanprodan/podinfo:test",
							Command: []string{
								"./podinfo",
								"--port=9898",
								"--port-metrics=9797",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "http-prom",
									ContainerPort: 9797,
									Protocol:      corev1.ProtocolTCP,
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

func newTestABTestDeployment() *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "abtest",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "abtest",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "abtest",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "podinfo",
							Image: "quay.io/stefanprodan/podinfo:test",
							Command: []string{
								"./podinfo",
								"--port=9898",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
									Protocol:      corev1.ProtocolTCP,
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

func newTestCanaryIngress() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			IngressRef: &flaggerv1.CrossNamespaceObjectReference{
				Name:       "podinfo",
				APIVersion: "extensions/v1beta1",
				Kind:       "Ingress",
			},
			Service: flaggerv1.CanaryService{
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
				},
			},
		},
	}
	return cd
}

func newTestIngress() *v1beta1.Ingress {
	return &v1beta1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "nginx",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: "app.example.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: "podinfo",
										ServicePort: intstr.FromInt(9898),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
