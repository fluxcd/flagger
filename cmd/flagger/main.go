package main

import (
	"flag"
	_ "github.com/istio/glog"
	clientset "github.com/stefanprodan/flagger/pkg/client/clientset/versioned"
	informers "github.com/stefanprodan/flagger/pkg/client/informers/externalversions"
	"github.com/stefanprodan/flagger/pkg/controller"
	"github.com/stefanprodan/flagger/pkg/logging"
	"github.com/stefanprodan/flagger/pkg/notifier"
	"github.com/stefanprodan/flagger/pkg/server"
	"github.com/stefanprodan/flagger/pkg/signals"
	"github.com/stefanprodan/flagger/pkg/version"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"time"
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
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&metricsServer, "metrics-server", "http://prometheus:9090", "Prometheus URL")
	flag.DurationVar(&controlLoopInterval, "control-loop-interval", 10*time.Second, "Kubernetes API sync interval")
	flag.StringVar(&logLevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&port, "port", "8080", "Port to listen on.")
	flag.StringVar(&slackURL, "slack-url", "", "Slack hook URL.")
	flag.StringVar(&slackUser, "slack-user", "flagger", "Slack user name.")
	flag.StringVar(&slackChannel, "slack-channel", "", "Slack channel.")
	flag.IntVar(&threadiness, "threadiness", 2, "Worker concurrency.")
	flag.BoolVar(&zapReplaceGlobals, "zap-replace-globals", false, "Whether to change the logging level of the global zap logger.")
	flag.StringVar(&zapEncoding, "zap-encoding", "json", "Zap logger encoding.")
}

func main() {
	flag.Parse()

	logger, err := logging.NewLoggerWithEncoding(logLevel, zapEncoding)
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

	istioClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building istio clientset: %v", err)
	}

	flaggerClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building example clientset: %s", err.Error())
	}

	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, time.Second*30)
	canaryInformer := flaggerInformerFactory.Flagger().V1alpha3().Canaries()

	logger.Infof("Starting flagger version %s revision %s", version.VERSION, version.REVISION)

	ver, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error calling Kubernetes API: %v", err)
	}

	logger.Infof("Connected to Kubernetes API %s", ver)

	ok, err := controller.CheckMetricsServer(metricsServer)
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

	c := controller.NewController(
		kubeClient,
		istioClient,
		flaggerClient,
		canaryInformer,
		controlLoopInterval,
		metricsServer,
		logger,
		slack,
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
