package main

import (
	"flag"
	"log"

	"time"

	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	"github.com/knative/pkg/signals"
	pd "github.com/stefanprodan/steerer/pkg/deployer"
	"github.com/stefanprodan/steerer/pkg/logging"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	namespace       string
	masterURL       string
	kubeconfig      string
	window          time.Duration
	promURL         string
	threshold       float64
	thresholdWindow string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	flag.StringVar(&namespace, "namespace", "", "Kubernetes namespace")
	flag.StringVar(&promURL, "prometheus", "https://prometheus.istio.weavedx.com", "Prometheus URL")
	flag.DurationVar(&window, "window", 10*time.Second, "wait interval between deployment rollouts")
	flag.Float64Var(&threshold, "threshold", 99, "HTTP request success rate threshold (1-99) to halt the rollout")
	flag.StringVar(&thresholdWindow, "interval", "1m", "HTTP request success rate query interval 30s 1m")
}

func main() {
	flag.Parse()

	logger, err := logging.NewLogger("debug")
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}
	defer logger.Sync()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building kubernetes clientset: %v", err)
	}

	istioClient, err := istioclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building shared clientset: %v", err)
	}

	ver, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error calling Kubernetes API: %v", err)
	}
	logger.Infof("Connected to Kubernetes API %v", ver)

	stopCh := signals.SetupSignalHandler()

	obs, err := pd.NewObserver(promURL, thresholdWindow)
	if err != nil {
		logger.Fatalf("Error parsing Prometheus URL: %v", err)
	}

	deployer := pd.NewDeployer(kubeClient, istioClient, obs, threshold, logger)
	deployer.Run(namespace)
	tickChan := time.NewTicker(window).C
	for {
		select {
		case <-tickChan:
			deployer.Run(namespace)
		case <-stopCh:
			return
		}
	}
}
