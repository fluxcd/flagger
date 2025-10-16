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
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceRouter_Create(t *testing.T) {
	mocks := newFixture(nil)
	mocks.canary.Spec.Service.Headless = true

	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		matchLabels: map[string]string{
			"name":         "podinfo",
			"test-label-1": "test-label-value-1",
		},
		labelSelector: "name",
		labelValue:    "podinfo",
	}
	appProtocol := "http"

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, len(canarySvc.Spec.Selector))
	assert.Equal(t, "podinfo", canarySvc.Spec.Selector["name"])
	assert.Equal(t, "test-label-value-1", canarySvc.Spec.Selector["test-label-1"])
	assert.Equal(t, &appProtocol, canarySvc.Spec.Ports[0].AppProtocol)
	assert.Equal(t, "http", canarySvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9898), canarySvc.Spec.Ports[0].Port)
	assert.Equal(t, "None", canarySvc.Spec.ClusterIP)

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, len(primarySvc.Spec.Selector))
	assert.Equal(t, "podinfo-primary", primarySvc.Spec.Selector["name"])
	assert.Equal(t, "test-label-value-1", primarySvc.Spec.Selector["test-label-1"])
	assert.Equal(t, "http", primarySvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9898), primarySvc.Spec.Ports[0].Port)
	assert.Equal(t, "None", primarySvc.Spec.ClusterIP)
}

func TestServiceRouter_TrafficDistribution(t *testing.T) {
	mocks := newFixture(nil)
	trafficDistribution := "PreferClose"
	mocks.canary.Spec.Service.TrafficDistribution = trafficDistribution

	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, canarySvc.Spec.TrafficDistribution)
	assert.Equal(t, trafficDistribution, *canarySvc.Spec.TrafficDistribution)

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, primarySvc.Spec.TrafficDistribution)
	assert.Equal(t, trafficDistribution, *primarySvc.Spec.TrafficDistribution)

	apexSvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, apexSvc.Spec.TrafficDistribution)
	assert.Equal(t, trafficDistribution, *apexSvc.Spec.TrafficDistribution)
}

func TestServiceRouter_Update(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canary, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	canaryClone := canary.DeepCopy()
	canaryClone.Spec.Service.PortName = "grpc"

	c, err := mocks.flaggerClient.FlaggerV1beta1().Canaries("default").Update(context.TODO(), canaryClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// apply changes
	err = router.Initialize(c)
	require.NoError(t, err)
	err = router.Reconcile(c)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "grpc", canarySvc.Spec.Ports[0].Name)
}

func TestServiceRouter_Undo(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)

	svcClone := canarySvc.DeepCopy()
	svcClone.Spec.Ports[0].Name = "http2-podinfo"
	svcClone.Spec.Ports[0].Port = 8080

	_, err = mocks.kubeClient.CoreV1().Services("default").Update(context.TODO(), svcClone, metav1.UpdateOptions{})
	require.NoError(t, err)

	// undo changes
	err = router.Initialize(mocks.canary)
	require.NoError(t, err)
	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	canarySvc, err = mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "http", canarySvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(9898), canarySvc.Spec.Ports[0].Port)
}

func TestServiceRouter_isOwnedByCanary(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
	}

	isController := new(bool)
	*isController = true

	tables := []struct {
		svc         *corev1.Service
		isOwned     bool
		hasOwnerRef bool
	}{
		// owned
		{
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podinfo",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "flagger.app/v1alpha3",
							Kind:       "Canary",
							Name:       "podinfo",
							Controller: isController,
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:     "http",
						Protocol: "TCP",
						Port:     8080,
					}},
					Selector: map[string]string{"app": "podinfo"},
				},
			}, isOwned: true, hasOwnerRef: true,
		},
		// Owner ref but kind not Canary
		{
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podinfo",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "flagger.app/v1alpha3",
							Kind:       "Deployment",
							Name:       "podinfo",
							Controller: isController,
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:     "http",
						Protocol: "TCP",
						Port:     8080,
					}},
					Selector: map[string]string{"app": "podinfo"},
				},
			}, isOwned: false, hasOwnerRef: false,
		},
		// Owner ref but name doesn't match
		{
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podinfo",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "flagger.app/v1alpha3",
							Kind:       "Canary",
							Name:       "notpodinfo",
							Controller: isController,
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:     "http",
						Protocol: "TCP",
						Port:     8080,
					}},
					Selector: map[string]string{"app": "podinfo"},
				},
			}, isOwned: false, hasOwnerRef: true,
		},
		// No ownerRef
		{
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podinfo",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:     "http",
						Protocol: "TCP",
						Port:     8080,
					}},
					Selector: map[string]string{"app": "podinfo"},
				},
			}, isOwned: false, hasOwnerRef: false,
		},
	}

	for _, table := range tables {
		hasOwnerRef, wasOwned := router.isOwnedByCanary(table.svc, mocks.canary.Name)
		if table.isOwned && !wasOwned {
			t.Error("Expected to be owned, but was not")
		} else if !table.isOwned && wasOwned {
			t.Error("Expected not to be owned but was")
		} else if table.hasOwnerRef && !hasOwnerRef {
			t.Error("Expected to contain OwnerReference but not present")
		} else if !table.hasOwnerRef && hasOwnerRef {
			t.Error("Expected not to have an OwnerReference but present")
		}
	}

}

func TestServiceRouter_Finalize(t *testing.T) {

	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		labelSelector: "app",
	}

	isController := new(bool)
	*isController = true

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podinfo",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "flagger.app/v1alpha3",
					Kind:       "Canary",
					Name:       "NotOwned",
					Controller: isController,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     9898,
			}},
			Selector: map[string]string{"app": "podinfo"},
		},
	}

	kubectlSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podinfo",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "flagger.app/v1alpha3",
					Kind:       "Canary",
					Name:       "NotOwned",
					Controller: isController,
				},
			},
			Annotations: map[string]string{
				kubectlAnnotation: `{"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"app":"podinfo"},"name":"podinfo","namespace":"test"},"spec":{"ports":[{"name":"http","port":9898,"protocol":"TCP","targetPort":9898}],"selector":{"app":"podinfo"},"type":"ClusterIP"}}`,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     9898,
			}},
			Selector: map[string]string{"app": "podinfo"},
		},
	}

	tables := []struct {
		router           *KubernetesDefaultRouter
		callSetupMethods bool
		shouldError      bool
		canary           *flaggerv1.Canary
		shouldMutate     bool
	}{
		// Won't reconcile since it is owned and would be garbage collected
		{router: router, callSetupMethods: true, shouldError: false, canary: mocks.canary, shouldMutate: false},
		// Service not found
		{router: &KubernetesDefaultRouter{kubeClient: fake.NewSimpleClientset(), logger: mocks.logger}, callSetupMethods: false, shouldError: true, canary: mocks.canary, shouldMutate: false},
		// Not owned
		{router: &KubernetesDefaultRouter{kubeClient: fake.NewSimpleClientset(svc), logger: mocks.logger}, callSetupMethods: false, shouldError: false, canary: mocks.canary, shouldMutate: true},
		// Kubectl annotation
		{router: &KubernetesDefaultRouter{kubeClient: fake.NewSimpleClientset(kubectlSvc), logger: mocks.logger}, callSetupMethods: false, shouldError: false, canary: mocks.canary, shouldMutate: true},
	}

	for _, table := range tables {

		if table.callSetupMethods {
			err := table.router.Initialize(table.canary)
			require.NoError(t, err)
			err = table.router.Reconcile(table.canary)
			require.NoError(t, err)
		}

		err := table.router.Finalize(table.canary)
		if table.shouldError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}

		svc, err := table.router.kubeClient.CoreV1().Services(table.canary.Namespace).Get(context.TODO(), table.canary.Name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				require.Equal(t, "http", svc.Spec.Ports[0].Name)
				require.Equal(t, 9898, svc.Spec.Ports[0].Port)

				if table.shouldMutate {
					require.Equal(t, table.canary.Name, svc.Spec.Selector["app"])
				} else {
					require.Equal(t, fmt.Sprintf("%s-primary", table.canary.Name), svc.Spec.Selector["app"])
				}
			}
		}
	}
}

func TestServiceRouter_InitializeMetadata(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		labelSelector: "app",
	}

	metadata := &flaggerv1.CustomMetadata{
		Labels:      map[string]string{"test": "test"},
		Annotations: map[string]string{"test": "test"},
	}

	mocks.canary.Spec.Service.Canary = metadata

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test", canarySvc.Annotations["test"])
	assert.Equal(t, "test", canarySvc.Labels["test"])
	assert.Equal(t, "podinfo-canary", canarySvc.Labels["app"])

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(primarySvc.Annotations))
	assert.Equal(t, "podinfo-primary", primarySvc.Labels["app"])
}

func TestServiceRouter_ReconcileMetadata(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		labelSelector: "app",
	}

	mocks.canary.Spec.Service.Apex = &flaggerv1.CustomMetadata{
		Labels:      map[string]string{"test": "test"},
		Annotations: map[string]string{"test": "test"},
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test", apexSvc.Annotations["test"])
	assert.Equal(t, "test", apexSvc.Labels["test"])
	assert.Equal(t, "podinfo", apexSvc.Labels["app"])

	canarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-canary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(canarySvc.Annotations))
	assert.Equal(t, "podinfo-canary", canarySvc.Labels["app"])

	primarySvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo-primary", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(primarySvc.Annotations))
	assert.Equal(t, "podinfo-primary", primarySvc.Labels["app"])

	mocks.canary.Spec.Service.Apex = &flaggerv1.CustomMetadata{
		Labels:      map[string]string{"test": "test1"},
		Annotations: map[string]string{"test1": "test"},
	}

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err = mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test", apexSvc.Annotations["test1"])
	assert.Equal(t, "test1", apexSvc.Labels["test"])
	assert.Equal(t, "podinfo", apexSvc.Labels["app"])
}

func TestServiceRouter_UnmanagedAnnotations(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		labelSelector: "app",
	}

	mocks.canary.Spec.Service.Apex = &flaggerv1.CustomMetadata{
		Annotations: map[string]string{"test": "expectedvalue"},
	}
	mocks.canary.Spec.Service.UnmanagedMetadata = &flaggerv1.UnmanagedMetadata{
		Annotations: []string{"unmanaged"},
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	clone := apexSvc.DeepCopy()
	clone.Annotations["unmanaged"] = "true"
	clone.Annotations["test"] = "newvalue"
	clone.Annotations["removable"] = "true"
	_, err = mocks.kubeClient.CoreV1().Services("default").Update(context.TODO(), clone, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err = mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "expectedvalue", apexSvc.Annotations["test"])
	assert.Equal(t, "true", apexSvc.Annotations["unmanaged"])
	_, ok := apexSvc.Annotations["removable"]
	assert.False(t, ok)
}

func TestServiceRouter_UnmanagedLabels(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		labelSelector: "app",
	}

	mocks.canary.Spec.Service.Apex = &flaggerv1.CustomMetadata{
		Labels: map[string]string{"test": "expectedvalue"},
	}
	mocks.canary.Spec.Service.UnmanagedMetadata = &flaggerv1.UnmanagedMetadata{
		Labels: []string{"unmanaged"},
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	clone := apexSvc.DeepCopy()
	clone.Labels["unmanaged"] = "true"
	clone.Labels["test"] = "newvalue"
	clone.Labels["removable"] = "true"
	_, err = mocks.kubeClient.CoreV1().Services("default").Update(context.TODO(), clone, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err = mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "expectedvalue", apexSvc.Labels["test"])
	assert.Equal(t, "true", apexSvc.Labels["unmanaged"])
	_, ok := apexSvc.Labels["removable"]
	assert.False(t, ok)
}

func TestServiceRouter_UnmanagedMetadata_AnnotationsAndLabels(t *testing.T) {
	mocks := newFixture(nil)
	router := &KubernetesDefaultRouter{
		kubeClient:    mocks.kubeClient,
		flaggerClient: mocks.flaggerClient,
		logger:        mocks.logger,
		labelSelector: "app",
	}

	mocks.canary.Spec.Service.Apex = &flaggerv1.CustomMetadata{
		Annotations: map[string]string{"test": "expectedvalue"},
		Labels:      map[string]string{"test": "expectedvalue"},
	}
	mocks.canary.Spec.Service.UnmanagedMetadata = &flaggerv1.UnmanagedMetadata{
		Annotations: []string{"unmanaged"},
		Labels:      []string{"unmanaged"},
	}

	err := router.Initialize(mocks.canary)
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err := mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	clone := apexSvc.DeepCopy()
	clone.Annotations["unmanaged"] = "true"
	clone.Annotations["test"] = "newvalue"
	clone.Annotations["removable"] = "true"
	clone.Labels["unmanaged"] = "true"
	clone.Labels["test"] = "newvalue"
	clone.Labels["removable"] = "true"
	_, err = mocks.kubeClient.CoreV1().Services("default").Update(context.TODO(), clone, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = router.Reconcile(mocks.canary)
	require.NoError(t, err)

	apexSvc, err = mocks.kubeClient.CoreV1().Services("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)

	// The result should be that the canary spec annotations should be changed back to configured canary value,
	// and the unmanaged annotation should remain unchanged.
	assert.Equal(t, "expectedvalue", apexSvc.Annotations["test"])
	assert.Equal(t, "true", apexSvc.Annotations["unmanaged"])
	_, ok := apexSvc.Annotations["removable"]
	assert.False(t, ok)

	assert.Equal(t, "expectedvalue", apexSvc.Labels["test"])
	assert.Equal(t, "true", apexSvc.Labels["unmanaged"])
	_, ok = apexSvc.Labels["removable"]
	assert.False(t, ok)
}
