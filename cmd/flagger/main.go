package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	semver "github.com/Masterminds/semver"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	informers "github.com/weaveworks/flagger/pkg/client/informers/externalversions"
	"github.com/weaveworks/flagger/pkg/controller"
	"github.com/weaveworks/flagger/pkg/logger"
	"github.com/weaveworks/flagger/pkg/metrics"
	"github.com/weaveworks/flagger/pkg/notifier"
	"github.com/weaveworks/flagger/pkg/router"
	"github.com/weaveworks/flagger/pkg/server"
	"github.com/weaveworks/flagger/pkg/signals"
	"github.com/weaveworks/flagger/pkg/version"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	_ "k8s.io/code-generator/cmd/client-gen/generators"
)

var (
	masterURL           string
	kubeconfig          string
	metricsServer       string
	controlLoopInterval time.Duration
	logLevel            string
	port                string
	slackURL            string
	slackUser           string
	slackChannel        string
	threadiness         int
	zapReplaceGlobals   bool
	zapEncoding         string
	namespace           string
	meshProvider        string
	selectorLabels      string
	ver                 bool
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&metricsServer, "metrics-server", "http://prometheus:9090", "Prometheus URL.")
	flag.DurationVar(&controlLoopInterval, "control-loop-interval", 10*time.Second, "Kubernetes API sync interval.")
	flag.StringVar(&logLevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&port, "port", "8080", "Port to listen on.")
	flag.StringVar(&slackURL, "slack-url", "", "Slack hook URL.")
	flag.StringVar(&slackUser, "slack-user", "flagger", "Slack user name.")
	flag.StringVar(&slackChannel, "slack-channel", "", "Slack channel.")
	flag.IntVar(&threadiness, "threadiness", 2, "Worker concurrency.")
	flag.BoolVar(&zapReplaceGlobals, "zap-replace-globals", false, "Whether to change the logging level of the global zap logger.")
	flag.StringVar(&zapEncoding, "zap-encoding", "json", "Zap logger encoding.")
	flag.StringVar(&namespace, "namespace", "", "Namespace that flagger would watch canary object.")
	flag.StringVar(&meshProvider, "mesh-provider", "istio", "Service mesh provider, can be istio, appmesh, supergloo, nginx or smi.")
	flag.StringVar(&selectorLabels, "selector-labels", "app,name,app.kubernetes.io/name", "List of pod labels that Flagger uses to create pod selectors.")
	flag.BoolVar(&ver, "version", false, "Print version")
}

func main() {
	flag.Parse()

	if ver {
		fmt.Println("Flagger version", version.VERSION, "revision ", version.REVISION)
		os.Exit(0)
	}

	logger, err := logger.NewLoggerWithEncoding(logLevel, zapEncoding)
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}
	if zapReplaceGlobals {
		zap.ReplaceGlobals(logger.Desugar())
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

	meshClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building mesh clientset: %v", err)
	}

	flaggerClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building flagger clientset: %s", err.Error())
	}

	flaggerInformerFactory := informers.NewSharedInformerFactoryWithOptions(flaggerClient, time.Second*30, informers.WithNamespace(namespace))

	canaryInformer := flaggerInformerFactory.Flagger().V1alpha3().Canaries()

	logger.Infof("Starting flagger version %s revision %s mesh provider %s", version.VERSION, version.REVISION, meshProvider)

	ver, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error calling Kubernetes API: %v", err)
	}

	k8sVersionConstraint := "^1.11.0"

	// We append -alpha.1 to the end of our version constraint so that prebuilds of later versions
	// are considered valid for our purposes, as well as some managed solutions like EKS where they provide
	// a version like `v1.12.6-eks-d69f1b`. It doesn't matter what the prelease value is here, just that it
	// exists in our constraint.
	semverConstraint, err := semver.NewConstraint(k8sVersionConstraint + "-alpha.1")
	if err != nil {
		logger.Fatalf("Error parsing kubernetes version constraint: %v", err)
	}

	k8sSemver, err := semver.NewVersion(ver.GitVersion)
	if err != nil {
		logger.Fatalf("Error parsing kubernetes version as a semantic version: %v", err)
	}

	if !semverConstraint.Check(k8sSemver) {
		logger.Fatalf("Unsupported version of kubernetes detected.  Expected %s, got %v", k8sVersionConstraint, ver)
	}

	labels := strings.Split(selectorLabels, ",")
	if len(labels) < 1 {
		logger.Fatalf("At least one selector label is required")
	}

	logger.Infof("Connected to Kubernetes API %s", ver)
	if namespace != "" {
		logger.Infof("Watching namespace %s", namespace)
	}

	observerFactory, err := metrics.NewFactory(metricsServer, meshProvider, 5*time.Second)
	if err != nil {
		logger.Fatalf("Error building prometheus client: %s", err.Error())
	}

	ok, err := observerFactory.Client.IsOnline()
	if ok {
		logger.Infof("Connected to metrics server %s", metricsServer)
	} else {
		logger.Errorf("Metrics server %s unreachable %v", metricsServer, err)
	}

	var slack *notifier.Slack
	if slackURL != "" {
		slack, err = notifier.NewSlack(slackURL, slackUser, slackChannel)
		if err != nil {
			logger.Errorf("Notifier %v", err)
		} else {
			logger.Infof("Slack notifications enabled for channel %s", slack.Channel)
		}
	}

	// start HTTP server
	go server.ListenAndServe(port, 3*time.Second, logger, stopCh)

	routerFactory := router.NewFactory(cfg, kubeClient, flaggerClient, logger, meshClient)

	c := controller.NewController(
		kubeClient,
		meshClient,
		flaggerClient,
		canaryInformer,
		controlLoopInterval,
		metricsServer,
		logger,
		slack,
		routerFactory,
		observerFactory,
		meshProvider,
		version.VERSION,
		labels,
	)

	flaggerInformerFactory.Start(stopCh)

	logger.Info("Waiting for informer caches to sync")
	for _, synced := range []cache.InformerSynced{
		canaryInformer.Informer().HasSynced,
	} {
		if ok := cache.WaitForCacheSync(stopCh, synced); !ok {
			logger.Fatalf("Failed to wait for cache sync")
		}
	}

	// start controller
	go func(ctrl *controller.Controller) {
		if err := ctrl.Run(threadiness, stopCh); err != nil {
			logger.Fatalf("Error running controller: %v", err)
		}
	}(c)

	<-stopCh
}
