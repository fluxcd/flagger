# Flagger

[Flagger](https://github.com/weaveworks/flagger) is a Kubernetes operator that automates the promotion of 
canary deployments using Istio, Linkerd, App Mesh, NGINX or Gloo routing for traffic shifting and Prometheus metrics for canary analysis. 
Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance indicators
like HTTP requests success rate, requests average duration and pods health.
Based on the KPIs analysis a canary is promoted or aborted and the analysis result is published to Slack or MS Teams.

## Prerequisites

* Kubernetes >= 1.11
* Prometheus >= 2.6

## Installing the Chart

Add Flagger Helm repository:

```console
$ helm repo add flagger https://flagger.app
```

Install Flagger's custom resource definitions:

```console
$ kubectl apply -f https://raw.githubusercontent.com/weaveworks/flagger/master/artifacts/flagger/crd.yaml
```

To install the chart with the release name `flagger` for Istio:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=istio-system \
    --set crd.create=false \
    --set meshProvider=istio \
    --set metricsServer=http://prometheus:9090
```

To install the chart with the release name `flagger` for Linkerd:

```console
$ helm upgrade -i flagger flagger/flagger \
    --namespace=linkerd \
    --set crd.create=false \
    --set meshProvider=linkerd \
    --set metricsServer=http://linkerd-prometheus:9090
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
`image.repository` | image repository | `weaveworks/flagger`
`image.tag` | image tag | `<VERSION>`
`image.pullPolicy` | image pull policy | `IfNotPresent`
`prometheus.install` | if `true`, installs Prometheus configured to scrape all pods in the custer including the App Mesh sidecar | `false`
`metricsServer` | Prometheus URL, used when `prometheus.install` is `false` | `http://prometheus.istio-system:9090`
`slack.url` | Slack incoming webhook | None
`slack.channel` | Slack channel | None
`slack.user` | Slack username | `flagger`
`msteams.url` | Microsoft Teams incoming webhook | None
`leaderElection.enabled` | leader election must be enabled when running more than one replica | `false`
`leaderElection.replicaCount` | number of replicas | `1`
`rbac.create` | if `true`, create and use RBAC resources | `true`
`rbac.pspEnabled` | If `true`, create and use a restricted pod security policy | `false`
`crd.create` | if `true`, create Flagger's CRDs | `true`
`resources.requests/cpu` | pod CPU request | `10m`
`resources.requests/memory` | pod memory request | `32Mi`
`resources.limits/cpu` | pod CPU limit | `1000m`
`resources.limits/memory` | pod memory limit | `512Mi`
`affinity` | node/pod affinities | None
`nodeSelector` | node labels for pod assignment | `{}`
`tolerations` | list of node taints to tolerate | `[]`

Specify each parameter using the `--set key=value[,key=value]` argument to `helm upgrade`. For example,

```console
$ helm upgrade -i flagger flagger/flagger \
  --namespace istio-system \
  --set crd.create=false \
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


