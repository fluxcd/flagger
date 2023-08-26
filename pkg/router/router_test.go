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

package router

import (
	a6v2 "github.com/fluxcd/flagger/pkg/apis/apisix/v2"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	appmesh "github.com/fluxcd/flagger/pkg/apis/appmesh"
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	gatewayapiv1 "github.com/fluxcd/flagger/pkg/apis/gatewayapi/v1beta1"
	istiov1alpha1 "github.com/fluxcd/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/fluxcd/flagger/pkg/apis/istio/v1alpha3"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
	"github.com/fluxcd/flagger/pkg/logger"
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
	apisixRoute := newTestApisixRoute()

	flaggerClient := fakeFlagger.NewSimpleClientset(
		canary,
		abtest,
		appmeshCanary,
		ingressCanary,
		apisixRoute,
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

func newTestApisixRoute() *a6v2.ApisixRoute {
	ar := &a6v2.ApisixRoute{
		TypeMeta: metav1.TypeMeta{APIVersion: a6v2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: a6v2.ApisixRouteSpec{HTTP: []a6v2.ApisixRouteHTTP{
			{
				Name: "method",
				Match: a6v2.ApisixRouteHTTPMatch{
					Hosts:   []string{"foobar.com"},
					Methods: []string{"GET"},
					Paths:   []string{"/*"},
				},
				Plugins: []a6v2.ApisixRoutePlugin{
					{
						Name:   "prometheus",
						Enable: true,
						Config: a6v2.ApisixRoutePluginConfig{
							"disable":     "false",
							"prefer_name": "true",
						},
					},
				},
				Backends: []a6v2.ApisixRouteHTTPBackend{
					{ServiceName: "podinfo",
						ServicePort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 80,
						}},
				},
			},
		},
		},
	}
	return ar
}

func newTestCanary() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Service: flaggerv1.CanaryService{
				Port:          9898,
				PortDiscovery: true,
				AppProtocol:   "http",
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
					{
						Name: "podinfo",
						Uri: &istiov1alpha1.StringMatch{
							Prefix: "/podinfo",
						},
						Method: &istiov1alpha1.StringMatch{
							Exact: "GET",
						},
						IgnoreUriCase: true,
					},
				},
				Retries: &istiov1alpha3.HTTPRetry{
					Attempts:      10,
					PerTryTimeout: "30s",
					RetryOn:       "connect-failure,gateway-error",
				},
				Gateways: []string{
					"istio/public-gateway",
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
			Annotations: map[string]string{
				appmesh.AccessLogAnnotation: appmesh.EnabledValue,
			},
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			Service: flaggerv1.CanaryService{
				Port:     9898,
				MeshName: "global",
				Hosts:    []string{"*"},
				Backends: []string{"backend.default"},
				Timeout:  "30s",
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
			TargetRef: flaggerv1.LocalObjectReference{
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
			TargetRef: flaggerv1.LocalObjectReference{
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
			Annotations: map[string]string{
				"kustomize.toolkit.fluxcd.io/checksum": "0a40893bfdc545d62125bd3e74eeb2ebaa7097c2",
			},
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			IngressRef: &flaggerv1.LocalObjectReference{
				Name:       "podinfo",
				APIVersion: "networking.k8s.io/v1",
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

func newTestIngress() *netv1.Ingress {
	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: netv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":          "nginx",
				"kustomize.toolkit.fluxcd.io/checksum": "0a40893bfdc545d62125bd3e74eeb2ebaa7097c2",
			},
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "app.example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "podinfo",
											Port: netv1.ServiceBackendPort{
												Number: 9898,
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
}

func newTestGatewayAPICanary() *flaggerv1.Canary {
	cd := &flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{
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
				GatewayRefs: []gatewayapiv1.ParentReference{
					{
						Name: "podinfo",
					},
				},
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
