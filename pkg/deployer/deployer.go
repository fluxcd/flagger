package deployer

import (
	"sync"

	"fmt"
	"strings"

	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

type Deployer struct {
	KubeClientSet          kubernetes.Interface
	IstioClientSet         istioclientset.Interface
	Logger                 *zap.SugaredLogger
	ProgressiveDeployments *sync.Map
}

type ProgressiveDeployment struct {
	Namespace        string
	Deployment       string
	DeploymentCanary string
	Service          string
	ServiceCanary    string
	VirtualService   string
}

func NewDeployer(kubeClient kubernetes.Interface, istioClient istioclientset.Interface, logger *zap.SugaredLogger) *Deployer {
	d := &Deployer{
		KubeClientSet:          kubeClient,
		IstioClientSet:         istioClient,
		Logger:                 logger,
		ProgressiveDeployments: new(sync.Map),
	}
	return d
}

func (d *Deployer) Run(ns string) {
	d.scanForDeployments(ns)
	d.ProgressiveDeployments.Range(func(key interface{}, value interface{}) bool {
		pd := value.(*ProgressiveDeployment)
		go d.advanceDeployment(pd)
		return true
	})
}

func (d *Deployer) scanForDeployments(namespace string) {
	annotation := "apps.weave.works/progressive"

	// scan for healthy deployments marked for progressive delivery
	deployments := make(map[string]appsv1.Deployment)
	depList, err := d.KubeClientSet.AppsV1().Deployments(namespace).List(v1.ListOptions{})
	if err != nil {
		d.Logger.Errorf("Get deployments failed: %v", err)
		return
	}
	for _, dep := range depList.Items {
		if val, ok := dep.Annotations[annotation]; ok {
			if val == "true" && !strings.Contains(dep.GetName(), "-canary") {
				if msg, healthy := getDeploymentStatus(&dep); healthy {
					deployments[dep.GetName()] = dep
				} else {
					d.Logger.Warnw(
						"Ignoring deployment",
						zap.String("name", dep.GetName()),
						zap.String("reason", msg))
				}
			}
		}
	}

	// sync deployments map
	d.ProgressiveDeployments.Range(func(key interface{}, value interface{}) bool {
		_, ok := deployments[key.(string)]
		if !ok {
			d.ProgressiveDeployments.Delete(key)
		}
		return true
	})

	for k, v := range deployments {
		if pd := d.makeProgressiveDeployment(&v); pd != nil {
			d.ProgressiveDeployments.Store(k, pd)
		}
	}
}

func (d *Deployer) makeProgressiveDeployment(dep *appsv1.Deployment) *ProgressiveDeployment {
	depCanaryName := fmt.Sprintf("%s-canary", dep.GetName())
	depCanary, err := d.KubeClientSet.AppsV1().Deployments(dep.Namespace).Get(depCanaryName, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Canary deployment %s not found", depCanaryName)
		return nil
	}

	svc, err := d.KubeClientSet.CoreV1().Services(dep.Namespace).Get(dep.GetName(), v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Service %s not found", dep.GetName())
		return nil
	}

	svcCanaryName := fmt.Sprintf("%s-canary", svc.GetName())
	svcCanary, err := d.KubeClientSet.CoreV1().Services(svc.Namespace).Get(svcCanaryName, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Canary service %s not found", svcCanaryName)
		return nil
	}

	vsvc, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(dep.Namespace).Get(dep.GetName(), v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Virtual Service %s not found", dep.GetName())
		return nil
	}

	return &ProgressiveDeployment{
		Namespace:        dep.Namespace,
		Deployment:       dep.GetName(),
		DeploymentCanary: depCanary.GetName(),
		Service:          svc.GetName(),
		ServiceCanary:    svcCanary.GetName(),
		VirtualService:   vsvc.GetName(),
	}
}

func (d *Deployer) advanceDeployment(pd *ProgressiveDeployment) {
	dep, err := d.KubeClientSet.AppsV1().Deployments(pd.Namespace).Get(pd.Deployment, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Deployment %s not found", pd.Deployment)
		return
	}
	if msg, healthy := getDeploymentStatus(dep); !healthy {
		d.Logger.Infof("Ignoring deployment %s %s", dep.GetName(), msg)
		return
	}

	canary, err := d.KubeClientSet.AppsV1().Deployments(pd.Namespace).Get(pd.DeploymentCanary, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Canary deployment %s not found", pd.DeploymentCanary)
		return
	}
	if msg, healthy := getDeploymentStatus(canary); !healthy {
		d.Logger.Infof("Ignoring deployment %s %s", canary.GetName(), msg)
		return
	}

	vsvc, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Get(pd.VirtualService, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Virtual Service %s not found", pd.VirtualService)
		return
	}

	shouldAdvance := false
	crAnnotation := "apps.weave.works/canary-revision"
	statusAnnotation := "apps.weave.works/canary-status"
	if val, ok := vsvc.Annotations[crAnnotation]; !ok {
		vsvc.Annotations[crAnnotation] = canary.ResourceVersion
		vsvc.Annotations[statusAnnotation] = "running"
		_, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Update(vsvc)
		if err != nil {
			d.Logger.Errorf("Virtual Service %s annotations update failed: %v", pd.VirtualService, err)
			return
		}
		shouldAdvance = true
	} else {
		if vsvc.Annotations[statusAnnotation] == "running" {
			shouldAdvance = true
		}
		if val != canary.ResourceVersion {
			vsvc.Annotations[crAnnotation] = canary.ResourceVersion
			vsvc.Annotations[statusAnnotation] = "running"
			_, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Update(vsvc)
			if err != nil {
				d.Logger.Errorf("Virtual Service %s annotations update failed: %v", pd.VirtualService, err)
				return
			}
			shouldAdvance = true
		}
	}
	if !shouldAdvance {
		return
	}

	var svcRoute istiov1alpha3.DestinationWeight
	var canaryRoute istiov1alpha3.DestinationWeight
	for _, http := range vsvc.Spec.Http {
		for _, route := range http.Route {
			if route.Destination.Host == pd.Service {
				svcRoute = route
			}
			if route.Destination.Host == pd.ServiceCanary {
				canaryRoute = route
			}
		}
	}

	vsvc, err = d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Get(pd.VirtualService, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Virtual Service %s not found", pd.VirtualService)
		return
	}

	if svcRoute.Weight == 0 && canaryRoute.Weight == 0 {
		d.Logger.Errorf("Virtual Service %s does not contain routes for %s and %s",
			pd.VirtualService, pd.Service, pd.ServiceCanary)
		return
	}

	if canaryRoute.Weight == 0 {
		d.Logger.Infof("Stating progressive deployment for %s", pd.Deployment)
	}

	if canaryRoute.Weight != 100 {
		svcRoute.Weight -= 10
		canaryRoute.Weight += 10
		vsvc.Spec.Http = []istiov1alpha3.HTTPRoute{
			{
				Route: []istiov1alpha3.DestinationWeight{svcRoute, canaryRoute},
			},
		}

		_, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Update(vsvc)
		if err != nil {
			d.Logger.Errorf("Virtual Service %s update failed: %v", pd.VirtualService, err)
			return
		} else {
			d.Logger.Infof("Advance deployment %s canary weight %v", pd.Deployment, canaryRoute.Weight)
		}

		if canaryRoute.Weight == 100 {
			dep.Spec.Template.Spec = canary.Spec.Template.Spec
			_, err = d.KubeClientSet.AppsV1().Deployments(dep.Namespace).Update(dep)
			if err != nil {
				d.Logger.Errorf("Deployment %s promotion failed: %v", dep.GetName(), err)
				return
			} else {
				d.Logger.Infof("Promote %s template spec to %s", canary.GetName(), dep.GetName())
			}
		}
	} else {
		svcRoute.Weight = 100
		canaryRoute.Weight = 0
		vsvc.Spec.Http = []istiov1alpha3.HTTPRoute{
			{
				Route: []istiov1alpha3.DestinationWeight{svcRoute, canaryRoute},
			},
		}
		vsvc.Annotations[statusAnnotation] = "finished"
		_, err = d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Update(vsvc)
		if err != nil {
			d.Logger.Errorf("Virtual Service %s annotations update failed: %v", pd.VirtualService, err)
			return
		}
		d.Logger.Infof("Deployment %s promotion complete", dep.GetName())

	}
}

func (d *Deployer) getServiceWorkloads(namespace string) {
	services, err := d.KubeClientSet.CoreV1().Services(namespace).List(v1.ListOptions{})
	if err != nil {
		d.Logger.Errorf("Get service from kubernetes cluster error:%v", err)
		return
	}

	for _, service := range services.Items {
		if service.Namespace == "default" && service.GetName() == "kubernetes" {
			continue
		}

		d.Logger.Infow("Found service",
			zap.String("service", service.GetName()),
		)

		set := labels.Set(service.Spec.Selector)

		if pods, err := d.KubeClientSet.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: set.AsSelector().String()}); err != nil {
			d.Logger.Errorf("List Pods of service[%s] error:%v", service.GetName(), err)
		} else {
			for _, pod := range pods.Items {
				rs, err := d.KubeClientSet.AppsV1().ReplicaSets(namespace).Get(pod.OwnerReferences[0].Name, v1.GetOptions{})
				if err != nil {
					d.Logger.Errorf("Get ReplicaSets for pod[%s] error:%v", pod.GetName(), err)
					continue
				}

				dep, err := d.KubeClientSet.AppsV1().Deployments(namespace).Get(rs.OwnerReferences[0].Name, v1.GetOptions{})
				if err != nil {
					d.Logger.Errorf("Get Deployment for ReplicaSets[%s] error:%v", rs.GetName(), err)
					continue
				}

				d.Logger.Infow("Found pod",
					zap.String("pod", pod.GetName()),
					zap.String("deployment", dep.GetName()),
					zap.Any("status", pod.Status.Phase),
				)
			}
		}
	}
}

func getDeploymentStatus(deployment *appsv1.Deployment) (string, bool) {
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		cond := getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
		if cond != nil && cond.Reason == "ProgressDeadlineExceeded" {
			return fmt.Sprintf("deployment %q exceeded its progress deadline", deployment.GetName()), false
		} else if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return fmt.Sprintf("waiting for rollout to finish: %d out of %d new replicas have been updated...",
				deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false
		} else if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("waiting for rollout to finish: %d old replicas are pending termination...",
				deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false
		} else if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("waiting for rollout to finish: %d of %d updated replicas are available...",
				deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false
		}
	} else {
		return "waiting for rollout to finish: observed deployment generation less then desired generation", false
	}

	return "ready", true
}

func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
