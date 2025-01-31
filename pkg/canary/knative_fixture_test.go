package canary

import (
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
	"github.com/fluxcd/flagger/pkg/logger"
	serving "knative.dev/serving/pkg/apis/serving/v1"
	knative "knative.dev/serving/pkg/client/clientset/versioned"
	fakeKnative "knative.dev/serving/pkg/client/clientset/versioned/fake"
)

type knativeControllerFixture struct {
	canary        *flaggerv1.Canary
	flaggerClient clientset.Interface
	knativeClient knative.Interface
	controller    KnativeController
	logger        *zap.SugaredLogger
}

func newKnativeServiceFixture(name string) knativeControllerFixture {
	canary := newKnativeControllerTestCanary(name)
	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	knativeClient := fakeKnative.NewSimpleClientset(newKnativeControllerTestService(name))

	logger, _ := logger.NewLogger("debug")

	ctrl := KnativeController{
		flaggerClient: flaggerClient,
		knativeClient: knativeClient,
		logger:        logger,
	}

	return knativeControllerFixture{
		canary:        canary,
		controller:    ctrl,
		logger:        logger,
		flaggerClient: flaggerClient,
		knativeClient: knativeClient,
	}
}

func newKnativeControllerTestService(name string) *serving.Service {
	s := &serving.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: serving.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: serving.ServiceSpec{
			ConfigurationSpec: serving.ConfigurationSpec{
				Template: serving.RevisionTemplateSpec{
					Spec: serving.RevisionSpec{
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "podinfo",
									Image: "quay.io/stefanprodan/podinfo:1.2.0",
								},
							},
						},
					},
				},
			},
		},
		Status: serving.ServiceStatus{
			ConfigurationStatusFields: serving.ConfigurationStatusFields{
				LatestCreatedRevisionName: "podinfo-00001",
			},
		},
	}

	return s
}

func newKnativeControllerTestCanary(name string) *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			Provider: "knative",
			TargetRef: flaggerv1.LocalObjectReference{
				Name:       name,
				APIVersion: "serving.knative.dev/v1",
				Kind:       "Service",
			},
			Analysis: &flaggerv1.CanaryAnalysis{},
		},
	}
	return cd
}
