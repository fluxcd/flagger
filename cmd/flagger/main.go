package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/transport"
	_ "k8s.io/code-generator/cmd/client-gen/generators"

	"github.com/weaveworks/flagger/pkg/canary"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	informers "github.com/weaveworks/flagger/pkg/client/informers/externalversions"
	"github.com/weaveworks/flagger/pkg/controller"
	"github.com/weaveworks/flagger/pkg/logger"
	"github.com/weaveworks/flagger/pkg/metrics/observers"
	"github.com/weaveworks/flagger/pkg/notifier"
	"github.com/weaveworks/flagger/pkg/router"
	"github.com/weaveworks/flagger/pkg/server"
	"github.com/weaveworks/flagger/pkg/signals"
	"github.com/weaveworks/flagger/pkg/version"
)

var (
	masterURL                string
	kubeconfig               string
	metricsServer            string
	controlLoopInterval      time.Duration
	logLevel                 string
	port                     string
	msteamsURL               string
	slackURL                 string
	slackUser                string
	slackChannel             string
	eventWebhook             string
	threadiness              int
	zapReplaceGlobals        bool
	zapEncoding              string
	namespace                string
	meshProvider             string
	selectorLabels           string
	ingressAnnotationsPrefix string
	enableLeaderElection     bool
	leaderElectionNamespace  string
	enableConfigTracking     bool
	ver                      bool
	kubeconfigServiceMesh    string
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
	flag.StringVar(&eventWebhook, "event-webhook", "", "Webhook for publishing flagger events")
	flag.StringVar(&msteamsURL, "msteams-url", "", "MS Teams incoming webhook URL.")
	flag.IntVar(&threadiness, "threadiness", 2, "Worker concurrency.")
	flag.BoolVar(&zapReplaceGlobals, "zap-replace-globals", false, "Whether to change the logging level of the global zap logger.")
	flag.StringVar(&zapEncoding, "zap-encoding", "json", "Zap logger encoding.")
	flag.StringVar(&namespace, "namespace", "", "Namespace that flagger would watch canary object.")
	flag.StringVar(&meshProvider, "mesh-provider", "istio", "Service mesh provider, can be istio, linkerd, appmesh, supergloo, nginx or smi.")
	flag.StringVar(&selectorLabels, "selector-labels", "app,name,app.kubernetes.io/name", "List of pod labels that Flagger uses to create pod selectors.")
	flag.StringVar(&ingressAnnotationsPrefix, "ingress-annotations-prefix", "nginx.ingress.kubernetes.io", "Annotations prefix for ingresses.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false, "Enable leader election.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "kube-system", "Namespace used to create the leader election config map.")
	flag.BoolVar(&enableConfigTracking, "enable-config-tracking", true, "Enable secrets and configmaps tracking.")
	flag.BoolVar(&ver, "version", false, "Print version")
	flag.StringVar(&kubeconfigServiceMesh, "kubeconfig-service-mesh", "", "Path to a kubeconfig for the service mesh control plane cluster.")
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

	logger.Infof("Starting flagger version %s revision %s mesh provider %s", version.VERSION, version.REVISION, meshProvider)

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building kubernetes clientset: %v", err)
	}

	flaggerClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building flagger clientset: %s", err.Error())
	}

	// use a remote cluster for routing if a service mesh kubeconfig is specified
	if kubeconfigServiceMesh == "" {
		kubeconfigServiceMesh = kubeconfig
	}
	cfgHost, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfigServiceMesh)
	if err != nil {
		logger.Fatalf("Error building host kubeconfig: %v", err)
	}

	meshClient, err := clientset.NewForConfig(cfgHost)
	if err != nil {
		logger.Fatalf("Error building mesh clientset: %v", err)
	}

	verifyCRDs(flaggerClient, logger)
	verifyKubernetesVersion(kubeClient, logger)
	infos := startInformers(flaggerClient, logger, stopCh)

	labels := strings.Split(selectorLabels, ",")
	if len(labels) < 1 {
		logger.Fatalf("At least one selector label is required")
	}

	if namespace != "" {
		logger.Infof("Watching namespace %s", namespace)
	}

	observerFactory, err := observers.NewFactory(metricsServer)
	if err != nil {
		logger.Fatalf("Error building prometheus client: %s", err.Error())
	}

	ok, err := observerFactory.Client.IsOnline()
	if ok {
		logger.Infof("Connected to metrics server %s", metricsServer)
	} else {
		logger.Errorf("Metrics server %s unreachable %v", metricsServer, err)
	}

	// setup Slack or MS Teams notifications
	notifierClient := initNotifier(logger)

	// start HTTP server
	go server.ListenAndServe(port, 3*time.Second, logger, stopCh)

	routerFactory := router.NewFactory(cfg, kubeClient, flaggerClient, ingressAnnotationsPrefix, logger, meshClient)

	var configTracker canary.Tracker
	if enableConfigTracking {
		configTracker = &canary.ConfigTracker{
			Logger:        logger,
			KubeClient:    kubeClient,
			FlaggerClient: flaggerClient,
		}
	} else {
		configTracker = &canary.NopTracker{}
	}

	canaryFactory := canary.NewFactory(kubeClient, flaggerClient, configTracker, labels, logger)

	c := controller.NewController(
		kubeClient,
		flaggerClient,
		infos,
		controlLoopInterval,
		logger,
		notifierClient,
		canaryFactory,
		routerFactory,
		observerFactory,
		meshProvider,
		version.VERSION,
		fromEnv("EVENT_WEBHOOK_URL", eventWebhook),
	)

	// leader election context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// prevents new requests when leadership is lost
	cfg.Wrap(transport.ContextCanceller(ctx, fmt.Errorf("the leader is shutting down")))

	// cancel leader election context on shutdown signals
	go func() {
		<-stopCh
		cancel()
	}()

	// wrap controller run
	runController := func() {
		if err := c.Run(threadiness, stopCh); err != nil {
			logger.Fatalf("Error running controller: %v", err)
		}
	}

	// run controller when this instance wins the leader election
	if enableLeaderElection {
		ns := leaderElectionNamespace
		if namespace != "" {
			ns = namespace
		}
		startLeaderElection(ctx, runController, ns, kubeClient, logger)
	} else {
		runController()
	}
}

func startInformers(flaggerClient clientset.Interface, logger *zap.SugaredLogger, stopCh <-chan struct{}) controller.Informers {
	flaggerInformerFactory := informers.NewSharedInformerFactoryWithOptions(flaggerClient, time.Second*30, informers.WithNamespace(namespace))

	logger.Info("Waiting for canary informer cache to sync")
	canaryInformer := flaggerInformerFactory.Flagger().V1beta1().Canaries()
	go canaryInformer.Informer().Run(stopCh)
	if ok := cache.WaitForNamedCacheSync("flagger", stopCh, canaryInformer.Informer().HasSynced); !ok {
		logger.Fatalf("failed to wait for cache to sync")
	}

	logger.Info("Waiting for metric template informer cache to sync")
	metricInformer := flaggerInformerFactory.Flagger().V1beta1().MetricTemplates()
	go metricInformer.Informer().Run(stopCh)
	if ok := cache.WaitForNamedCacheSync("flagger", stopCh, metricInformer.Informer().HasSynced); !ok {
		logger.Fatalf("failed to wait for cache to sync")
	}

	logger.Info("Waiting for alert provider informer cache to sync")
	alertInformer := flaggerInformerFactory.Flagger().V1beta1().AlertProviders()
	go alertInformer.Informer().Run(stopCh)
	if ok := cache.WaitForNamedCacheSync("flagger", stopCh, alertInformer.Informer().HasSynced); !ok {
		logger.Fatalf("failed to wait for cache to sync")
	}

	return controller.Informers{
		CanaryInformer: canaryInformer,
		MetricInformer: metricInformer,
		AlertInformer:  alertInformer,
	}
}

func startLeaderElection(ctx context.Context, run func(), ns string, kubeClient kubernetes.Interface, logger *zap.SugaredLogger) {
	configMapName := "flagger-leader-election"
	id, err := os.Hostname()
	if err != nil {
		logger.Fatalf("Error running controller: %v", err)
	}
	id = id + "_" + string(uuid.NewUUID())

	lock, err := resourcelock.New(
		resourcelock.ConfigMapsResourceLock,
		ns,
		configMapName,
		kubeClient.CoreV1(),
		kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id,
		},
	)
	if err != nil {
		logger.Fatalf("Error running controller: %v", err)
	}

	logger.Infof("Starting leader election id: %s configmap: %s namespace: %s", id, configMapName, ns)
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   60 * time.Second,
		RenewDeadline:   15 * time.Second,
		RetryPeriod:     5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logger.Info("Acting as elected leader")
				run()
			},
			OnStoppedLeading: func() {
				logger.Infof("Leadership lost")
				os.Exit(1)
			},
			OnNewLeader: func(identity string) {
				if identity != id {
					logger.Infof("Another instance has been elected as leader: %v", identity)
				}
			},
		},
	})
}

func initNotifier(logger *zap.SugaredLogger) (client notifier.Interface) {
	provider := "slack"
	notifierURL := fromEnv("SLACK_URL", slackURL)
	if msteamsURL != "" || os.Getenv("MSTEAMS_URL") != "" {
		provider = "msteams"
		notifierURL = fromEnv("MSTEAMS_URL", msteamsURL)
	}
	notifierFactory := notifier.NewFactory(notifierURL, slackUser, slackChannel)

	var err error
	client, err = notifierFactory.Notifier(provider)
	if err != nil {
		logger.Errorf("Notifier %v", err)
	} else if len(notifierURL) > 30 {
		logger.Infof("Notifications enabled for %s", notifierURL[0:30])
	}
	return
}

func fromEnv(envVar string, defaultVal string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultVal
}

func verifyCRDs(flaggerClient clientset.Interface, logger *zap.SugaredLogger) {
	_, err := flaggerClient.FlaggerV1beta1().Canaries(namespace).List(context.TODO(), metav1.ListOptions{Limit: 1})
	if err != nil {
		logger.Fatalf("Canary CRD is not registered %v", err)
	}

	_, err = flaggerClient.FlaggerV1beta1().MetricTemplates(namespace).List(context.TODO(), metav1.ListOptions{Limit: 1})
	if err != nil {
		logger.Fatalf("MetricTemplate CRD is not registered %v", err)
	}

	_, err = flaggerClient.FlaggerV1beta1().AlertProviders(namespace).List(context.TODO(), metav1.ListOptions{Limit: 1})
	if err != nil {
		logger.Fatalf("AlertProvider CRD is not registered %v", err)
	}
}

func verifyKubernetesVersion(kubeClient kubernetes.Interface, logger *zap.SugaredLogger) {
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

	logger.Infof("Connected to Kubernetes API %s", ver)
}
