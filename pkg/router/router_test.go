package router

import (
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
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

type fakeClients struct {
	canary        *flaggerv1.Canary
	abtest        *flaggerv1.Canary
	appmeshCanary *flaggerv1.Canary
	ingressCanary *flaggerv1.Canary
	kubeClient    kubernetes.Interface
	meshClient    clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func setupfakeClients() fakeClients {
	canary := newMockCanary()
	abtest := newMockABTest()
	appmeshCanary := newMockCanaryAppMesh()
	ingressCanary := newMockCanaryIngress()
	flaggerClient := fakeFlagger.NewSimpleClientset(canary, abtest, appmeshCanary, ingressCanary)

	kubeClient := fake.NewSimpleClientset(newMockDeployment(), newMockABTestDeployment(), newMockIngress())

	meshClient := fakeFlagger.NewSimpleClientset()
	logger, _ := logger.NewLogger("debug")

	return fakeClients{
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

func newMockCanaryAppMesh() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "appmesh",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: hpav1.CrossVersionObjectReference{
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
			}, CanaryAnalysis: flaggerv1.CanaryAnalysis{
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

func newMockCanary() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Service: flaggerv1.CanaryService{
				Port: 9898,
				Headers: &istiov1alpha3.Headers{
					Request: &istiov1alpha3.HeaderOperations{
						Add: map[string]string{
							"x-envoy-upstream-rq-timeout-ms": "15000",
						},
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
			}, CanaryAnalysis: flaggerv1.CanaryAnalysis{
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

func newMockMirror() *flaggerv1.Canary {
	cd := newMockCanary()
	cd.Spec.CanaryAnalysis.Mirror = true
	return cd
}

func newMockABTest() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "abtest",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: hpav1.CrossVersionObjectReference{
				Name:       "abtest",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Service: flaggerv1.CanaryService{
				Port:     9898,
				MeshName: "global",
			}, CanaryAnalysis: flaggerv1.CanaryAnalysis{
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

func newMockDeployment() *appsv1.Deployment {
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
							Image: "quay.io/stefanprodan/podinfo:1.4.0",
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

func newMockABTestDeployment() *appsv1.Deployment {
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
							Image: "quay.io/stefanprodan/podinfo:1.4.0",
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

func newMockCanaryIngress() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			IngressRef: &hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "extensions/v1beta1",
				Kind:       "Ingress",
			},
			Service: flaggerv1.CanaryService{
				Port: 9898,
			}, CanaryAnalysis: flaggerv1.CanaryAnalysis{
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

func newMockIngress() *v1beta1.Ingress {
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
