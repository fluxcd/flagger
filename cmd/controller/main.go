package main

import (
	"flag"
	"log"
	"time"

	"github.com/golang/glog"
	sharedclientset "github.com/knative/pkg/client/clientset/versioned"
	"github.com/knative/pkg/signals"
	clientset "github.com/stefanprodan/steerer/pkg/client/clientset/versioned"
	informers "github.com/stefanprodan/steerer/pkg/client/informers/externalversions"
	"github.com/stefanprodan/steerer/pkg/controller"
	"github.com/stefanprodan/steerer/pkg/logging"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	masterURL  string
	kubeconfig string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

func main() {
	flag.Parse()

	logger, err := logging.NewLogger("debug")
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}
	defer logger.Sync()

	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building kubernetes clientset: %v", err)
	}

	sharedClient, err := sharedclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building shared clientset: %v", err)
	}

	rolloutClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}

	rolloutInformerFactory := informers.NewSharedInformerFactory(rolloutClient, time.Second*30)
	rolloutInformer := rolloutInformerFactory.Apps().V1beta1().Rollouts()

	ver, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error calling Kubernetes API: %v", err)
	}
	logger.Infof("Kubernetes version %v", ver)

	c := controller.NewController(
		kubeClient,
		sharedClient,
		rolloutClient,
		rolloutInformer,
		logger,
	)

	rolloutInformerFactory.Start(stopCh)

	logger.Info("Waiting for informer caches to sync")
	for _, synced := range []cache.InformerSynced{
		rolloutInformer.Informer().HasSynced,
	} {
		if ok := cache.WaitForCacheSync(stopCh, synced); !ok {
			logger.Fatalf("failed to wait for cache")
		}
	}

	go func(ctrl *controller.Controller) {
		if runErr := ctrl.Run(2, stopCh); runErr != nil {
			logger.Fatalf("Error running controller: %v", runErr)
		}
	}(c)

	<-stopCh
}
