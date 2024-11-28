package canary

import (
	"go.uber.org/zap"
	hpav2 "k8s.io/api/autoscaling/v2"
	hpav2beta2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	keda "github.com/fluxcd/flagger/pkg/apis/keda/v1alpha1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
	"github.com/fluxcd/flagger/pkg/logger"
)

type scalerReconcilerFixture struct {
	canary           *flaggerv1.Canary
	kubeClient       kubernetes.Interface
	flaggerClient    clientset.Interface
	scalerReconciler ScalerReconciler
	logger           *zap.SugaredLogger
}

type scalerConfig struct {
	targetName  string
	excludeObjs []string
	scaler      string
}

func newScalerReconcilerFixture(cfg scalerConfig) scalerReconcilerFixture {
	canary := newDeploymentControllerTestCanary(canaryConfigs{targetName: cfg.targetName})
	flaggerClient := fakeFlagger.NewSimpleClientset(
		canary,
		newScaledObject(),
	)

	kubeClient := fake.NewSimpleClientset(
		newScalerReconcilerTestHPAV2(),
		newScalerReconcilerTestHPAV2Beta2(),
	)
	for _, obj := range cfg.excludeObjs {
		if obj == "HPAV2" {
			kubeClient.Tracker().Delete(schema.GroupVersionResource{
				Group:    "autoscaling",
				Version:  "v2",
				Resource: "horizontalpodautoscalers",
			}, "default", "podinfo")
		}
		if obj == "HPAV2Beta2" {
			kubeClient.Tracker().Delete(schema.GroupVersionResource{
				Group:    "autoscaling",
				Version:  "v2beta2",
				Resource: "horizontalpodautoscalers",
			}, "default", "podinfo")
		}
	}

	logger, _ := logger.NewLogger("debug")
	var scalerReconciler ScalerReconciler

	if cfg.scaler == "HorizontalPodAutoscaler" {
		scalerReconciler = &HPAReconciler{
			kubeClient:         kubeClient,
			flaggerClient:      flaggerClient,
			logger:             logger,
			includeLabelPrefix: []string{"app.kubernetes.io"},
		}
	}
	if cfg.scaler == "ScaledObject" {
		scalerReconciler = &ScaledObjectReconciler{
			kubeClient:         kubeClient,
			flaggerClient:      flaggerClient,
			logger:             logger,
			includeLabelPrefix: []string{"app.kubernetes.io"},
		}
	}

	return scalerReconcilerFixture{
		canary:           canary,
		kubeClient:       kubeClient,
		flaggerClient:    flaggerClient,
		scalerReconciler: scalerReconciler,
		logger:           logger,
	}
}

func newScalerReconcilerTestHPAV2Beta2() *hpav2beta2.HorizontalPodAutoscaler {
	h := &hpav2beta2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{APIVersion: hpav2beta2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: hpav2beta2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: hpav2beta2.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Metrics: []hpav2beta2.MetricSpec{
				{
					Type: "Resource",
					Resource: &hpav2beta2.ResourceMetricSource{
						Name: "cpu",
						Target: hpav2beta2.MetricTarget{
							AverageUtilization: int32p(99),
						},
					},
				},
			},
		},
	}

	return h
}

func newScalerReconcilerTestHPAV2() *hpav2.HorizontalPodAutoscaler {
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
						Name: "cpu",
						Target: hpav2.MetricTarget{
							AverageUtilization: int32p(99),
						},
					},
				},
			},
		},
	}

	return h
}

func newScaledObject() *keda.ScaledObject {
	so := &keda.ScaledObject{
		TypeMeta: metav1.TypeMeta{APIVersion: keda.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
			Annotations: map[string]string{
				"scaledobject.keda.sh/transfer-hpa-ownership": "true",
			},
		},
		Spec: keda.ScaledObjectSpec{
			ScaleTargetRef: &keda.ScaleTarget{
				Name: "podinfo",
			},
			Advanced: &keda.AdvancedConfig{
				HorizontalPodAutoscalerConfig: &keda.HorizontalPodAutoscalerConfig{
					Name: "podinfo",
				},
			},
			PollingInterval: int32p(10),
			MinReplicaCount: int32p(1),
			MaxReplicaCount: int32p(4),
			Triggers: []keda.ScaleTriggers{
				{
					Type:       "prometheus",
					MetricType: "AverageValue",
					Metadata: map[string]string{
						"serverAddress": "http://flagger-prometheus.projectcontour:9090",
						"metricName":    "http_requests_total",
						"query":         `sum(rate(http_requests_total{app="podinfo-canary"}[2m]))`,
						"threshold":     "100",
					},
				},
			},
		},
	}

	return so
}
