package router

import (
	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha3"
	istiov1alpha3 "github.com/stefanprodan/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/stefanprodan/flagger/pkg/client/clientset/versioned/fake"
	"github.com/stefanprodan/flagger/pkg/logging"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeClients struct {
	canary        *v1alpha3.Canary
	kubeClient    kubernetes.Interface
	istioClient   clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

func setupfakeClients() fakeClients {
	canary := newMockCanary()
	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(
		newMockDeployment(),
	)

	istioClient := fakeFlagger.NewSimpleClientset()
	logger, _ := logging.NewLogger("debug")

	return fakeClients{
		canary:        canary,
		kubeClient:    kubeClient,
		istioClient:   istioClient,
		flaggerClient: flaggerClient,
		logger:        logger,
	}
}

func newMockCanary() *v1alpha3.Canary {
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
			Service: v1alpha3.CanaryService{
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
