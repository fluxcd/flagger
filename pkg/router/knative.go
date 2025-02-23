package router

import (
	"context"
	"fmt"
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	knative "knative.dev/serving/pkg/client/clientset/versioned"
	"slices"
)

type KnativeRouter struct {
	knativeClient knative.Interface
}

func (kr *KnativeRouter) Reconcile(canary *flaggerv1.Canary) error {
	fmt.Printf("Reconcile %s", canary)
	return nil
}
func (kr *KnativeRouter) SetRoutes(cd *flaggerv1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error {
	service, err := kr.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	primaryName, exists := service.Annotations["flagger.app/primary-revision"]
	if !exists {
		return fmt.Errorf("service %s.%s annotation not found", cd.Spec.TargetRef.Name, cd.Namespace)
	}
	traffic := make([]v1.TrafficTarget, 2)
	canaryPercent := int64(canaryWeight)
	primaryPercent := int64(primaryWeight)
	latestRevision := true
	traffic[0] = v1.TrafficTarget{
		LatestRevision: &latestRevision,
		Percent:        &canaryPercent,
	}
	traffic[1] = v1.TrafficTarget{
		RevisionName: primaryName,
		Percent:      &primaryPercent,
	}
	service.Spec.Traffic = traffic
	service, err = kr.knativeClient.ServingV1().Services(cd.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s update query error %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return nil
}
func (kr *KnativeRouter) GetRoutes(cd *flaggerv1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, error error) {
	service, err := kr.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		error = fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
		return
	}
	primaryName, exists := service.Annotations["flagger.app/primary-revision"]
	if !exists {
		error = fmt.Errorf("service %s.%s annotation not found", cd.Spec.TargetRef.Name, cd.Namespace)
		return
	}
	canaryRevisionIdx := slices.IndexFunc(service.Status.Traffic, func(target v1.TrafficTarget) bool {
		return *target.LatestRevision
	})
	primaryRevisionIdx := slices.IndexFunc(service.Status.Traffic, func(target v1.TrafficTarget) bool {
		return target.RevisionName == primaryName
	})
	if canaryRevisionIdx == -1 || primaryRevisionIdx == -1 {
		error = fmt.Errorf("TODO: Could not find primary or canary revision")
		return
	}
	return int(*service.Status.Traffic[primaryRevisionIdx].Percent), int(*service.Status.Traffic[canaryRevisionIdx].Percent), false, nil
}
func (kr *KnativeRouter) Finalize(canary *flaggerv1.Canary) error {
	fmt.Printf("Finalize %s", canary)
	return nil
}
