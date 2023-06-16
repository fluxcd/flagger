# Flagger

[Flagger](https://github.com/fluxcd/flagger) is a progressive delivery tool that automates the release process
for applications running on Kubernetes. It reduces the risk of introducing a new software version in production
by gradually shifting traffic to the new version while measuring metrics and running conformance tests.

Flagger implements several deployment strategies (Canary releases, A/B testing, Blue/Green mirroring)
and integrates with various Kubernetes ingress controllers, service mesh and monitoring solutions.

Flagger is a [Cloud Native Computing Foundation](https://cncf.io/) project
and part of [Flux](https://fluxcd.io) family of GitOps tools.

## Prerequisites

* Kubernetes >= 1.19

## Installing the Chart

Add Flagger Helm repository:

```console
$ helm repo add flagger https://flagger.app
```

Install Flagger's custom resource definitions:

```console
$ kubectl apply -f https://raw.githubusercontent.com/fluxcd/flagger/main/artifacts/flagger/crd.yaml
```

To install Flagger for **Istio**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=istio-system \
    --set meshProvider=istio \
    --set metricsServer=http://prometheus:9090
```

To install Flagger for **Linkerd** (requires Linkerd Viz extension):

```console
# Note that linkerdAuthPolicy.create=true is only required for Linkerd 2.12 and
# later
$ helm upgrade -i flagger flagger/flagger \
    --namespace=flagger-system \
    --set meshProvider=linkerd \
    --set metricsServer=http://prometheus.linkerd-viz:9090 \
    --set linkerdAuthPolicy.create=true
```

To install Flagger for **AWS App Mesh**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=appmesh-system \
    --set meshProvider=appmesh:v1beta2 \
    --set metricsServer=http://appmesh-prometheus:9090
```


To install Flagger for **Open Service Mesh** (requires OSM to have been installed with Prometheus):

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=osm-system \
    --set meshProvider=osm \
    --set metricsServer=http://osm-prometheus.osm-system.svc:7070
```

To install Flagger for **Kuma Service Mesh** (requires Kuma to have been installed with Prometheus):

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=kuma-system \
    --set meshProvider=kuma \
    --set metricsServer=http://prometheus-server.kuma-metrics:80
```

To install Flagger and Prometheus for **NGINX** Ingress (requires controller metrics enabled):

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=ingress-nginx \
    --set meshProvider=nginx \
    --set prometheus.install=true
```

To install Flagger and Prometheus for **Gloo** (no longer requires Gloo discovery):

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
    --set ingressClass=contour \
    --set prometheus.install=true
```

To install Flagger and Prometheus for **Traefik**:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=traefik \
    --set prometheus.install=true \
    --set meshProvider=traefik
```

The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `flagger` deployment:

```console
$ helm delete flagger
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the Flagger chart and their default values.

| Parameter                            | Description                                                                                                                                        | Default                               |
|--------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------|
| `image.repository`                   | Image repository                                                                                                                                   | `ghcr.io/fluxcd/flagger`              |
| `image.tag`                          | Image tag                                                                                                                                          | `<VERSION>`                           |
| `image.pullPolicy`                   | Image pull policy                                                                                                                                  | `IfNotPresent`                        |
| `logLevel`                           | Log level                                                                                                                                          | `info`                                |
| `metricsServer`                      | Prometheus URL, used when `prometheus.install` is `false`                                                                                          | `http://prometheus.istio-system:9090` |
| `prometheus.install`                 | If `true`, installs Prometheus configured to scrape all pods in the custer                                                                         | `false`                               |
| `prometheus.retention`               | Prometheus data retention                                                                                                                          | `2h`                                  |
| `selectorLabels`                     | List of labels that Flagger uses to create pod selectors                                                                                           | `app,name,app.kubernetes.io/name`     |
| `serviceMonitor.enabled`             | If `true`, creates service and serviceMonitor for monitoring Flagger metrics                                                                                           | `false`     |
| `serviceMonitor.honorLabels`         | If `true`, label conflicts are resolved by keeping label values from the scraped data and ignoring the conflicting server-side labels              | `false`                               |
| `configTracking.enabled`             | If `true`, flagger will track changes in Secrets and ConfigMaps referenced in the target deployment                                                | `true`                                |
| `eventWebhook`                       | If set, Flagger will publish events to the given webhook                                                                                           | None                                  |
| `slack.url`                          | Slack incoming webhook                                                                                                                             | None                                  |
| `slack.proxyUrl`                     | Slack proxy url                                                                                                                                    | None                                  |
| `slack.channel`                      | Slack channel                                                                                                                                      | None                                  |
| `slack.user`                         | Slack username                                                                                                                                     | `flagger`                             |
| `msteams.url`                        | Microsoft Teams incoming webhook                                                                                                                   | None                                  |
| `msteams.proxyUrl`                   | Microsoft Teams proxy url                                                                                                                          | None                                  |
| `clusterName`                        | When specified, Flagger will add the cluster name to alerts                                                                                        | `""`                                  |
| `podMonitor.enabled`                 | If `true`, create a PodMonitor for [monitoring the metrics](https://docs.flagger.app/usage/monitoring#metrics)                                     | `false`                               |
| `podMonitor.namespace`               | Namespace where the PodMonitor is created                                                                                                          | the same namespace                    |
| `podMonitor.interval`                | Interval at which metrics should be scraped                                                                                                        | `15s`                                 |
| `podMonitor.podMonitor`              | Additional labels to add to the PodMonitor                                                                                                         | `{}`                                  |
| `leaderElection.enabled`             | If `true`, Flagger will run in HA mode                                                                                                             | `false`                               |
| `leaderElection.replicaCount`        | Number of replicas                                                                                                                                 | `1`                                   |
| `serviceAccount.create`              | If `true`, Flagger will create service account                                                                                                     | `true`                                |
| `serviceAccount.name`                | The name of the service account to create or use. If not set and `serviceAccount.create` is `true`, a name is generated using the Flagger fullname | `""`                                  |
| `serviceAccount.annotations`         | Annotations for service account                                                                                                                    | `{}`                                  |
| `ingressAnnotationsPrefix`           | Annotations prefix for ingresses                                                                                                                   | `custom.ingress.kubernetes.io`        |
| `includeLabelPrefix`                 | List of prefixes of labels that are copied when creating primary deployments or daemonsets. Use * to include all                                   | `""`                                  |
| `rbac.create`                        | If `true`, create and use RBAC resources                                                                                                           | `true`                                |
| `rbac.pspEnabled`                    | If `true`, create and use a restricted pod security policy                                                                                         | `false`                               |
| `crd.create`                         | If `true`, create Flagger's CRDs (should be enabled for Helm v2 only)                                                                              | `false`                               |
| `resources.requests/cpu`             | Pod CPU request                                                                                                                                    | `10m`                                 |
| `resources.requests/memory`          | Pod memory request                                                                                                                                 | `32Mi`                                |
| `resources.limits/cpu`               | Pod CPU limit                                                                                                                                      | `1000m`                               |
| `resources.limits/memory`            | Pod memory limit                                                                                                                                   | `512Mi`                               |
| `affinity`                           | Node/pod affinities                                                                                                                                | prefer spread across hosts            |
| `nodeSelector`                       | Node labels for pod assignment                                                                                                                     | `{}`                                  |
| `threadiness`                        | Number of controller workers                                                                                                                       | `2`                                   |
| `tolerations`                        | List of node taints to tolerate                                                                                                                    | `[]`                                  |
| `controlplane.kubeconfig.secretName` | The name of the Kubernetes secret containing the service mesh control plane kubeconfig                                                             | None                                  |
| `controlplane.kubeconfig.key`        | The name of Kubernetes secret data key that contains the service mesh control plane kubeconfig                                                     | `kubeconfig`                          |
| `ingressAnnotationsPrefix`           | Annotations prefix for NGINX ingresses                                                                                                             | None                                  |
| `ingressClass`                       | Ingress class used for annotating HTTPProxy objects, e.g. `contour`                                                                                | None                                  |
| `podPriorityClassName`               | PriorityClass name for pod priority configuration                                                                                                  | ""                                    |
| `podDisruptionBudget.enabled`        | A PodDisruptionBudget will be created if `true`                                                                                                    | `false`                               |
| `podDisruptionBudget.minAvailable`   | The minimal number of available replicas that will be set in the PodDisruptionBudget                                                               | `1`                                   |
| `podDisruptionBudget.minAvailable`   | The minimal number of available replicas that will be set in the PodDisruptionBudget                                                               | `1`                                   |
| `noCrossNamespaceRefs`               | If `true`, cross namespace references to custom resources will be disabled                                                                         | `false`                               |
| `namespace`                          | When specified, Flagger will restrict itself to watching Canary objects from that namespace                                                                   | `""`                                  |

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
