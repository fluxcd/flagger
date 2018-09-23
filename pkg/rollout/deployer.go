package rollout

import (
	"fmt"
	"strings"
	"sync"

	"github.com/fatih/color"
	istiov1alpha3 "github.com/knative/pkg/apis/istio/v1alpha3"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	"github.com/stefanprodan/steerer/pkg/logging"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	enabledAnnotation  = "apps.weave.works/progressive"
	revisionAnnotation = "apps.weave.works/progressive-revision"
	statusAnnotation   = "apps.weave.works/progressive-status"
)

type Deployer struct {
	KubeClientSet          kubernetes.Interface
	IstioClientSet         istioclientset.Interface
	Logger                 *zap.SugaredLogger
	ProgressiveDeployments *sync.Map
	Observer               *Observer
	Threshold              float64
}

type ProgressiveDeployment struct {
	Namespace        string
	Deployment       string
	DeploymentCanary string
	Service          string
	ServiceCanary    string
	VirtualService   string
}

func NewDeployer(
	kubeClient kubernetes.Interface,
	istioClient istioclientset.Interface,
	observer *Observer,
	threshold float64,
	logger *zap.SugaredLogger,
) *Deployer {
	d := &Deployer{
		KubeClientSet:          kubeClient,
		IstioClientSet:         istioClient,
		Logger:                 logger,
		ProgressiveDeployments: new(sync.Map),
		Observer:               observer,
		Threshold:              threshold,
	}
	return d
}

func (d *Deployer) Run(namespace string) {
	d.scanForDeployments(namespace)
	d.ProgressiveDeployments.Range(func(key interface{}, value interface{}) bool {
		pd := value.(*ProgressiveDeployment)
		go d.advanceDeployment(pd)
		return true
	})
}

// scan for deployments marked for progressive rollout
func (d *Deployer) scanForDeployments(namespace string) {
	deployments := make(map[string]appsv1.Deployment)
	depList, err := d.KubeClientSet.AppsV1().Deployments(namespace).List(v1.ListOptions{})
	if err != nil {
		d.Logger.Errorf("Get deployments failed: %v", err)
		return
	}
	for _, dep := range depList.Items {
		if val, ok := dep.Annotations[enabledAnnotation]; ok {
			if val == "true" && !strings.Contains(dep.GetName(), "-canary") {
				deployments[dep.GetName()] = dep
			}
		}
	}

	if len(deployments) < 1 {
		d.Logger.Debugf(
			"no deployments found with the annotation %s='true' in namespace %s",
			enabledAnnotation,
			namespace,
		)
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
		d.Logger.Errorf("Deployment %s.%s not found", pd.Deployment, pd.Namespace)
		return
	}
	if msg, healthy := getDeploymentStatus(dep); !healthy {
		d.Logger.Infof("Ignoring deployment %s.%s %s", dep.GetName(), dep.Namespace, msg)
		logging.Console(
			fmt.Sprintf(
				"rollout halted %s.%s",
				dep.GetName(),
				dep.Namespace,
			), color.YellowString(msg))
		return
	}

	canary, err := d.KubeClientSet.AppsV1().Deployments(pd.Namespace).Get(pd.DeploymentCanary, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Canary deployment %s not found", pd.DeploymentCanary)
		return
	}

	// ignore deployment if canary relicas are zero or deployment is not healthy
	if canary.Spec.Replicas == nil || *canary.Spec.Replicas == 0 {
		return
	}
	if msg, healthy := getDeploymentStatus(canary); !healthy {
		d.Logger.Infof("Ignoring deployment %s.%s %s", canary.GetName(), canary.Namespace, msg)
		logging.Console(
			fmt.Sprintf(
				"rollout halted %s.%s",
				canary.GetName(),
				canary.Namespace,
			), color.YellowString(msg))
		return
	}

	vsvc, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Get(pd.VirtualService, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Virtual Service %s.%s not found", pd.VirtualService, pd.Namespace)
		return
	}

	shouldAdvance := false
	if val, ok := vsvc.Annotations[revisionAnnotation]; !ok {
		vsvc.Annotations[revisionAnnotation] = canary.ResourceVersion
		vsvc.Annotations[statusAnnotation] = "running"
		_, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Update(vsvc)
		if err != nil {
			d.Logger.Errorf("Virtual Service %s.%s annotations update failed: %v", pd.VirtualService, pd.Namespace, err)
			return
		}
		shouldAdvance = true
	} else {
		if vsvc.Annotations[statusAnnotation] == "running" {
			shouldAdvance = true
		}
		if val != canary.ResourceVersion {
			vsvc.Annotations[revisionAnnotation] = canary.ResourceVersion
			vsvc.Annotations[statusAnnotation] = "running"
			_, err := d.IstioClientSet.NetworkingV1alpha3().VirtualServices(pd.Namespace).Update(vsvc)
			if err != nil {
				d.Logger.Errorf("Virtual Service %s.%s annotations update failed: %v", pd.VirtualService, pd.Namespace, err)
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
		d.Logger.Errorf("Virtual Service %s.%s not found", pd.VirtualService, pd.Namespace)
		return
	}

	if svcRoute.Weight == 0 && canaryRoute.Weight == 0 {
		d.Logger.Errorf("Virtual Service %s.%s does not contain routes for %s and %s",
			pd.VirtualService, pd.Namespace, pd.Service, pd.ServiceCanary)
		return
	}

	// skip HTTP error rate check when there is no traffic
	if canaryRoute.Weight == 0 {
		d.Logger.Infof("Stating progressive deployment for %s.%s", pd.Deployment, pd.Namespace)
		depMetric, _ := d.checkSuccessRate(pd.Namespace, pd.DeploymentCanary)
		canaryMetric, _ := d.checkSuccessRate(pd.Namespace, pd.Deployment)
		logging.Console(fmt.Sprintf(
			"starting rolling deployment from %s.%s to %s.%s",
			pd.Deployment, pd.Namespace, pd.DeploymentCanary, pd.Namespace))
		logging.Console(
			fmt.Sprintf(
				"success rate for %s.%s is",
				pd.Deployment,
				pd.Namespace,
			),
			color.YellowString("%.2f%%", depMetric))
		logging.Console(
			fmt.Sprintf(
				"success rate for %s.%s is",
				pd.DeploymentCanary,
				pd.Namespace,
			),
			color.YellowString("%.2f%%", canaryMetric))
	} else {
		if val, ok := d.checkSuccessRate(pd.Namespace, pd.DeploymentCanary); !ok {
			d.Logger.Warnf("%s.%s rollout halted due to low HTTP success rate %v threshold %v",
				pd.DeploymentCanary, pd.Namespace, val, d.Threshold)
			logging.Console(
				fmt.Sprintf(
					"rollout halted %s.%s success rate",
					pd.DeploymentCanary,
					pd.Namespace,
				),
				color.RedString("%.2f%%", val),
				"<",
				color.GreenString("%v%%", d.Threshold))
			return
		}
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
			d.Logger.Errorf("Virtual Service %s.%s update failed: %v", pd.VirtualService, pd.Namespace, err)
			return
		} else {
			d.Logger.Infof("Advance deployment %s.%s weight %v", pd.DeploymentCanary, pd.Namespace, canaryRoute.Weight)
			logging.Console(
				fmt.Sprintf(
					"advancing deployment %s.%s",
					pd.DeploymentCanary,
					pd.Namespace,
				),
				"weight",
				color.GreenString(
					"%v",
					canaryRoute.Weight,
				))
		}

		if canaryRoute.Weight == 100 {
			d.Logger.Infof("Copying %s.%s template spec to %s.%s",
				canary.GetName(), canary.Namespace, dep.GetName(), dep.Namespace)
			logging.Console(
				fmt.Sprintf(
					"copying %s.%s template spec to %s.%s",
					pd.DeploymentCanary,
					pd.Namespace,
					pd.Deployment,
					pd.Namespace,
				))
			dep.Spec.Template.Spec = canary.Spec.Template.Spec
			_, err = d.KubeClientSet.AppsV1().Deployments(dep.Namespace).Update(dep)
			if err != nil {
				d.Logger.Errorf("Deployment %s.%s promotion failed: %v", dep.GetName(), dep.Namespace, err)
				return
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
			d.Logger.Errorf("Virtual Service %s.%s annotations update failed: %v", pd.VirtualService, pd.Namespace, err)
			return
		}
		d.Logger.Infof("%s.%s promotion complete! Scaling down %s.%s",
			dep.GetName(), dep.Namespace, canary.GetName(), canary.Namespace)
		d.scaleToZeroCanary(pd)
		logging.Console(fmt.Sprintf(
			"scaling %s.%s to zero",
			pd.DeploymentCanary,
			pd.Namespace,
		))
		logging.Console(
			color.GreenString(
				"%s.%s promotion completed",
				pd.DeploymentCanary,
				pd.Namespace,
			))
	}
}

func (d *Deployer) checkSuccessRate(namespace string, name string) (float64, bool) {
	val, err := d.Observer.GetDeploymentSuccessRate(namespace, name)
	if err != nil {
		d.Logger.Errorf("Observer Prometheus query error: %v", err)
		return 0, false
	}

	return val, val > d.Threshold

}

func (d *Deployer) scaleToZeroCanary(pd *ProgressiveDeployment) {
	canary, err := d.KubeClientSet.AppsV1().Deployments(pd.Namespace).Get(pd.DeploymentCanary, v1.GetOptions{})
	if err != nil {
		d.Logger.Errorf("Deployment %s.%s not found", pd.DeploymentCanary, pd.Namespace)
		return
	}
	//HPA https://github.com/kubernetes/kubernetes/pull/29212
	canary.Spec.Replicas = int32p(0)
	_, err = d.KubeClientSet.AppsV1().Deployments(canary.Namespace).Update(canary)
	if err != nil {
		d.Logger.Errorf("Scaling down %s.%s failed: %v", canary.GetName(), canary.Namespace, err)
		return
	}
}

func (d *Deployer) deleteCanary(pd *ProgressiveDeployment) {
	err := d.KubeClientSet.AppsV1().Deployments(pd.Namespace).Delete(pd.DeploymentCanary, &v1.DeleteOptions{})
	if err != nil {
		d.Logger.Errorf("Deleting deployment %s.%s failed: %v", pd.DeploymentCanary, pd.Namespace, err)
		return
	}
	err = d.KubeClientSet.CoreV1().Services(pd.Namespace).Delete(pd.ServiceCanary, &v1.DeleteOptions{})
	if err != nil {
		d.Logger.Errorf("Deleting service %s.%s failed: %v", pd.ServiceCanary, pd.Namespace, err)
		return
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

func int32p(i int32) *int32 {
	return &i
}
