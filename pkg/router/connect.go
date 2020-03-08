package router

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	consulapi "github.com/hashicorp/consul/api"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// ConsulConnectRouter is managing Consul connect splitters
type ConsulConnectRouter struct {
	kubeClient    kubernetes.Interface
	consulClient  *consulapi.Client
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Reconcile creates or updates the Istio virtual service and destination rules
func (cr *ConsulConnectRouter) Reconcile(canary *flaggerv1.Canary) error {
	services, _, err := cr.consulClient.Catalog().Services(&consulapi.QueryOptions{
		Datacenter: "dc1",
	})
	if err != nil {
		return err
	}

	for service := range services {
		println(service)
	}

	return nil
}

// GetRoutes returns the destinations weight for primary and canary
func (cr *ConsulConnectRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	return
}

// SetRoutes updates the destinations weight for primary and canary
func (cr *ConsulConnectRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
) error {
	return nil
}
