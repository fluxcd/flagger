package main

import (
	"flag"
	"log"
	"time"

	"fmt"

	"os"

	"github.com/fatih/color"
	istioclientset "github.com/knative/pkg/client/clientset/versioned"
	"github.com/knative/pkg/signals"
	"github.com/stefanprodan/steerer/pkg/logging"
	"github.com/stefanprodan/steerer/pkg/rollout"
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
	os.Setenv("console", "true")
	logger, err := logging.NewLogger("error")
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

	obs, err := rollout.NewObserver(promURL, thresholdWindow)
	if err != nil {
		logger.Fatalf("Error parsing Prometheus URL: %v", err)
	}

	fmt.Println(
		"starting progressive deployment engine control loop",
		color.GreenString("%vs", window.Seconds()),
		"threshold",
		color.GreenString("%v%%", threshold),
		"range",
		color.GreenString("%v", thresholdWindow))
	deployer := rollout.NewDeployer(kubeClient, istioClient, obs, threshold, logger)
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
