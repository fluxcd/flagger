package router

import (
	"context"
	"fmt"
	"slices"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	serving "knative.dev/serving/pkg/apis/serving/v1"
	knative "knative.dev/serving/pkg/client/clientset/versioned"
)

type KnativeRouter struct {
	knativeClient knative.Interface
	logger        *zap.SugaredLogger
}

func (kr *KnativeRouter) Reconcile(canary *flaggerv1.Canary) error {
	service, err := kr.knativeClient.ServingV1().Services(canary.Namespace).Get(context.TODO(), canary.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Knative Service %s.%s get query error: %w", canary.Spec.TargetRef.Name, canary.Namespace, err)
	}

	if _, ok := service.Annotations["flagger.app/primary-revision"]; !ok {
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		service.Annotations["flagger.app/primary-revision"] = service.Status.LatestCreatedRevisionName
		_, err = kr.knativeClient.ServingV1().Services(canary.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("Knative Service %s.%s update query error: %w", canary.Spec.TargetRef.Name, canary.Namespace, err)
		}
		kr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Knative Service %s.%s updated", service.Name, service.Namespace)
	}

	return nil
}

func (kr *KnativeRouter) SetRoutes(cd *flaggerv1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error {
	service, err := kr.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Knative Service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}

	primaryName, exists := service.Annotations["flagger.app/primary-revision"]
	if !exists {
		return fmt.Errorf("Knative Service %s.%s annotation not found", cd.Spec.TargetRef.Name, cd.Namespace)
	}

	canaryPercent := int64(canaryWeight)
	primaryPercent := int64(primaryWeight)
	latestRevision := true
	traffic := []serving.TrafficTarget{
		{
			LatestRevision: &latestRevision,
			Percent:        &canaryPercent,
		},
		{
			RevisionName: primaryName,
			Percent:      &primaryPercent,
		},
	}
	service.Spec.Traffic = traffic

	service, err = kr.knativeClient.ServingV1().Services(cd.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Knative Service %s.%s update query error %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
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

	canaryRevisionIdx := slices.IndexFunc(service.Status.Traffic, func(target serving.TrafficTarget) bool {
		return *target.LatestRevision
	})
	primaryRevisionIdx := slices.IndexFunc(service.Status.Traffic, func(target serving.TrafficTarget) bool {
		return target.RevisionName == primaryName
	})

	if canaryRevisionIdx == -1 || primaryRevisionIdx == -1 {
		error = fmt.Errorf("Knative Service %s.%s traffic spec invalid", cd.Spec.TargetRef.Name, cd.Namespace)
		return
	}
	if service.Status.Traffic[primaryRevisionIdx].Percent == nil {
		error = fmt.Errorf("Knative Service %s.%s primary revision traffic percent does not exist", cd.Spec.TargetRef.Name, cd.Namespace)
		return
	}
	if service.Status.Traffic[canaryRevisionIdx].Percent == nil {
		error = fmt.Errorf("Knative Service %s.%s canary revision traffic percent does not exist", cd.Spec.TargetRef.Name, cd.Namespace)
		return
	}

	return int(*service.Status.Traffic[primaryRevisionIdx].Percent), int(*service.Status.Traffic[canaryRevisionIdx].Percent), false, nil
}

func (kr *KnativeRouter) Finalize(canary *flaggerv1.Canary) error {
	service, err := kr.knativeClient.ServingV1().Services(canary.Namespace).Get(context.TODO(), canary.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Knative Service %s.%s get query error: %w", canary.Spec.TargetRef.Name, canary.Namespace, err)
	}

	if _, ok := service.Annotations["flagger.app/primary-revision"]; ok {
		delete(service.Annotations, "flagger.app/primary-revision")
		_, err = kr.knativeClient.ServingV1().Services(canary.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("Knative Service %s.%s update query error: %w", canary.Spec.TargetRef.Name, canary.Namespace, err)
		}
		kr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Knative Service %s.%s updated", service.Name, service.Namespace)
	}

	return nil
}
