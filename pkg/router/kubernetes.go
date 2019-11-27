package router

import (
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// KubernetesDeploymentRouter is managing ClusterIP services
type KubernetesRouter interface {
	// Reconcile creates or updates K8s services to prepare for the canary release
	Reconcile(canary *flaggerv1.Canary) error
}

func buildService(canary *flaggerv1.Canary, name string, src *corev1.Service) *corev1.Service {
	svc := src.DeepCopy()
	svc.ObjectMeta.Name = name
	svc.ObjectMeta.Namespace = canary.Namespace
	svc.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(canary, schema.GroupVersionKind{
			Group:   flaggerv1.SchemeGroupVersion.Group,
			Version: flaggerv1.SchemeGroupVersion.Version,
			Kind:    flaggerv1.CanaryKind,
		}),
	}
	_, exists := svc.ObjectMeta.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	if exists {
		// Leaving this results in updates from flagger to this svc never succeed due to resourceVersion mismatch:
		//   Operation cannot be fulfilled on services "mysvc-canary": the object has been modified; please apply your changes to the latest version and try again
		delete(svc.ObjectMeta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	}

	return svc
}
