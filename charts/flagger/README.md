# Flagger

[Flagger](https://github.com/weaveworks/flagger) is an operator that automates the release process of applications on Kubernetes. 

Flagger can run automated application analysis, testing, promotion and rollback for the following deployment strategies:
* Canary Release (progressive traffic shifting)
* A/B Testing (HTTP headers and cookies traffic routing)
* Blue/Green (traffic switching and mirroring)

Flagger works with service mesh solutions (Istio, Linkerd, AWS App Mesh) and with Kubernetes ingress controllers (NGINX, Gloo, Contour).
Flagger can be configured to send alerts to various chat platforms such as Slack, Microsoft Teams, Discord and Rocket.

## Prerequisites

* Kubernetes >= 1.14

## Installing the Chart

Add Flagger Helm repository:

```console
$ helm repo add flagger https://flagger.app
```

Install Flagger's custom resource definitions:

```console
$ kubectl apply -f https://raw.githubusercontent.com/weaveworks/flagger/master/artifacts/flagger/crd.yaml
```

To install Flagger for **Istio**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=istio-system \
    --set meshProvider=istio \
    --set metricsServer=http://prometheus:9090
```

To install Flagger for **Linkerd**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=linkerd \
    --set meshProvider=linkerd \
    --set metricsServer=http://linkerd-prometheus:9090
```

To install Flagger for **AWS App Mesh**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=appmesh-system \
    --set meshProvider=appmesh \
    --set metricsServer=http://appmesh-prometheus:9090
```

To install Flagger and Prometheus for **NGINX** Ingress (requires controller metrics enabled):

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=ingress-nginx \
    --set meshProvider=nginx \
    --set prometheus.install=true
```

To install Flagger and Prometheus for **Gloo** (requires Gloo discovery enabled):

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=gloo-system \
    --set meshProvider=gloo \
    --set prometheus.install=true
```

To install Flagger and Prometheus for **Contour**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=projectcontour \
    --set meshProvider=contour \
    --set prometheus.install=true
```

The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `flagger` deployment:

```console
$ helm delete --purge flagger
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the Flagger chart and their default values.

Parameter | Description | Default
--- | --- | ---
`image.repository` | Image repository | `weaveworks/flagger`
`image.tag` | Image tag | `<VERSION>`
`image.pullPolicy` | Image pull policy | `IfNotPresent`
`logLevel` | Log level | `info`
`prometheus.install` | If `true`, installs Prometheus configured to scrape all pods in the custer including the App Mesh sidecar | `false`
`metricsServer` | Prometheus URL, used when `prometheus.install` is `false` | `http://prometheus.istio-system:9090`
`selectorLabels` | List of labels that Flagger uses to create pod selectors | `app,name,app.kubernetes.io/name`
`configTracking.enabled` | If `true`, flagger will track changes in Secrets and ConfigMaps referenced in the target deployment | `true`
`eventWebhook` | If set, Flagger will publish events to the given webhook | None
`slack.url` | Slack incoming webhook | None
`slack.channel` | Slack channel | None
`slack.user` | Slack username | `flagger`
`msteams.url` | Microsoft Teams incoming webhook | None
`podMonitor.enabled` | If `true`, create a PodMonitor for [monitoring the metrics](https://docs.flagger.app/usage/monitoring#metrics) | `false`
`podMonitor.namespace` | Namespace where the PodMonitor is created | the same namespace 
`podMonitor.interval` | Interval at which metrics should be scraped | `15s` 
`podMonitor.podMonitor` | Additional labels to add to the PodMonitor | `{}`
`leaderElection.enabled` | If `true`, Flagger will run in HA mode | `false`
`leaderElection.replicaCount` | Number of replicas | `1`
`serviceAccount.create` | If `true`, Flagger will create service account | `true`
`serviceAccount.name` | The name of the service account to create or use. If not set and `serviceAccount.create` is `true`, a name is generated using the Flagger fullname | `""`
`serviceAccount.annotations` | Annotations for service account | `{}`
`ingressAnnotationsPrefix` | Annotations prefix for ingresses | `custom.ingress.kubernetes.io`
`rbac.create` | If `true`, create and use RBAC resources | `true`
`rbac.pspEnabled` | If `true`, create and use a restricted pod security policy | `false`
`crd.create` | If `true`, create Flagger's CRDs (should be enabled for Helm v2 only) | `false`
`resources.requests/cpu` | Pod CPU request | `10m`
`resources.requests/memory` | Pod memory request | `32Mi`
`resources.limits/cpu` | Pod CPU limit | `1000m`
`resources.limits/memory` | Pod memory limit | `512Mi`
`affinity` | Node/pod affinities | None
`nodeSelector` | Node labels for pod assignment | `{}`
`tolerations` | List of node taints to tolerate | `[]`
`istio.kubeconfig.secretName` | The name of the Kubernetes secret containing the Istio shared control plane kubeconfig | None
`istio.kubeconfig.key` | The name of Kubernetes secret data key that contains the Istio control plane kubeconfig | `kubeconfig`

Specify each parameter using the `--set key=value[,key=value]` argument to `helm upgrade`. For example,

```console
$ helm upgrade -i flagger flagger/flagger \
  --namespace flagger-system \
  --set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
  --set slack.channel=general
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm upgrade -i flagger flagger/flagger \
  --namespace istio-system \
  -f values.yaml
```

> **Tip**: You can use the default [values.yaml](values.yaml)


