package main

import (
	"flag"
	"log"
	"time"

	sharedclientset "github.com/knative/pkg/client/clientset/versioned"
	sharedscheme "github.com/knative/pkg/client/clientset/versioned/scheme"
	sharedinformers "github.com/knative/pkg/client/informers/externalversions"
	"github.com/knative/pkg/signals"
	"github.com/stefanprodan/steerer/pkg/controller"
	"github.com/stefanprodan/steerer/pkg/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
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

	sharedscheme.AddToScheme(scheme.Scheme)

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	sharedInformerFactory := sharedinformers.NewSharedInformerFactory(sharedClient, time.Second*30)

	coreServiceInformer := kubeInformerFactory.Core().V1().Services()
	virtualServiceInformer := sharedInformerFactory.Networking().V1alpha3().VirtualServices()

	ver, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error calling Kubernetes API: %v", err)
	}
	logger.Infof("Kubernetes version %v", ver)

	opts := v1.ListOptions{}
	list, err := sharedClient.NetworkingV1alpha3().VirtualServices("demo").List(opts)
	if err != nil {
		logger.Fatalf("Error building shared clientset: %v", err)
	}
	logger.Infof("VirtualServices %v", len(list.Items))

	c := controller.NewController(
		kubeClient,
		sharedClient,
		logger,
		coreServiceInformer,
		virtualServiceInformer,
	)

	kubeInformerFactory.Start(stopCh)
	sharedInformerFactory.Start(stopCh)

	logger.Info("Waiting for informer caches to sync")
	for i, synced := range []cache.InformerSynced{
		coreServiceInformer.Informer().HasSynced,
		virtualServiceInformer.Informer().HasSynced,
	} {
		if ok := cache.WaitForCacheSync(stopCh, synced); !ok {
			logger.Fatalf("failed to wait for cache at index %v to sync", i)
		}
	}

	go func(ctrl *controller.Controller) {
		if runErr := ctrl.Run(2, stopCh); runErr != nil {
			logger.Fatalf("Error running controller: %v", runErr)
		}
	}(c)

	<-stopCh
}
