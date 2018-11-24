package controller

import (
	"testing"

	"github.com/stefanprodan/flagger/pkg/apis/flagger/v1alpha1"
	fakeFlagger "github.com/stefanprodan/flagger/pkg/client/clientset/versioned/fake"
	"github.com/stefanprodan/flagger/pkg/logging"
	appsv1 "k8s.io/api/apps/v1"
	hpav1 "k8s.io/api/autoscaling/v1"
	hpav2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestCanary() *v1alpha1.Canary {
	cd := &v1alpha1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "podinfo",
		},
		Spec: v1alpha1.CanarySpec{
			TargetRef: hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			AutoscalerRef: hpav1.CrossVersionObjectReference{
				Name:       "podinfo",
				APIVersion: "autoscaling/v2beta1",
				Kind:       "HorizontalPodAutoscaler",
			}, Service: v1alpha1.CanaryService{
				Port: 9898,
			}, CanaryAnalysis: v1alpha1.CanaryAnalysis{
				Threshold:  10,
				StepWeight: 10,
				MaxWeight:  50,
				Metrics: []v1alpha1.CanaryMetric{
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

func TestCanaryDeployer_Sync(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep.Spec.Template.Spec.Containers[0].Image
	if primaryImage != sourceImage {
		t.Errorf("Got image %s wanted %s", primaryImage, sourceImage)
	}

	hpaPrimary, err := kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if hpaPrimary.Spec.ScaleTargetRef.Name != depPrimary.Name {
		t.Errorf("Got HPA target %s wanted %s", hpaPrimary.Spec.ScaleTargetRef.Name, depPrimary.Name)
	}
}

func TestCanaryDeployer_IsNewSpec(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	dep2 := newTestDeploymentUpdated()

	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	isNew, err := deployer.IsNewSpec(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !isNew {
		t.Errorf("Got %v wanted %v", isNew, true)
	}
}

func TestCanaryDeployer_Promote(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	dep2 := newTestDeploymentUpdated()

	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = kubeClient.AppsV1().Deployments("default").Update(dep2)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.Promote(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	depPrimary, err := kubeClient.AppsV1().Deployments("default").Get("podinfo-primary", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	primaryImage := depPrimary.Spec.Template.Spec.Containers[0].Image
	sourceImage := dep2.Spec.Template.Spec.Containers[0].Image
	if primaryImage != sourceImage {
		t.Errorf("Got image %s wanted %s", primaryImage, sourceImage)
	}
}

func TestCanaryDeployer_IsReady(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.IsReady(canary)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestCanaryDeployer_SetFailedChecks(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.SetFailedChecks(canary, 1)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := flaggerClient.FlaggerV1alpha1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.FailedChecks != 1 {
		t.Errorf("Got %v wanted %v", res.Status.FailedChecks, 1)
	}
}

func TestCanaryDeployer_SetState(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.SetState(canary, "running")
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := flaggerClient.FlaggerV1alpha1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.State != "running" {
		t.Errorf("Got %v wanted %v", res.Status.State, "running")
	}
}

func TestCanaryDeployer_SyncStatus(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	status := v1alpha1.CanaryStatus{
		State:        "running",
		FailedChecks: 2,
	}
	err = deployer.SyncStatus(canary, status)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, err := flaggerClient.FlaggerV1alpha1().Canaries("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Status.State != status.State {
		t.Errorf("Got state %v wanted %v", res.Status.State, status.State)
	}

	if res.Status.FailedChecks != status.FailedChecks {
		t.Errorf("Got failed checks %v wanted %v", res.Status.FailedChecks, status.FailedChecks)
	}
}

func TestCanaryDeployer_Scale(t *testing.T) {
	canary := newTestCanary()
	dep := newTestDeployment()
	hpa := newTestHPA()

	flaggerClient := fakeFlagger.NewSimpleClientset(canary)

	kubeClient := fake.NewSimpleClientset(dep, hpa)

	logger, _ := logging.NewLogger("debug")
	deployer := &CanaryDeployer{
		flaggerClient: flaggerClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}

	err := deployer.Sync(canary)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = deployer.Scale(canary, 2)

	c, err := kubeClient.AppsV1().Deployments("default").Get("podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if *c.Spec.Replicas != 2 {
		t.Errorf("Got replicas %v wanted %v", *c.Spec.Replicas, 2)
	}

}
