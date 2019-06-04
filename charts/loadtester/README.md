# Flagger load testing service

[Flagger's](https://github.com/weaveworks/flagger) load testing service is based on 
[rakyll/hey](https://github.com/rakyll/hey) 
and can be used to generates traffic during canary analysis when configured as a webhook.

## Prerequisites

* Kubernetes >= 1.11

## Installing the Chart

Add Flagger Helm repository:

```console
helm repo add flagger https://flagger.app
```

To install the chart with the release name `flagger-loadtester`:

```console
helm upgrade -i flagger-loadtester flagger/loadtester
```

The command deploys Grafana on the Kubernetes cluster in the default namespace.

> **Tip**: Note that the namespace where you deploy the load tester should have the Istio or App Mesh sidecar injection enabled

The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `flagger-loadtester` deployment:

```console
helm delete --purge flagger-loadtester
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the load tester chart and their default values.

Parameter | Description | Default
--- | --- | ---
`image.repository` | Image repository | `quay.io/stefanprodan/flagger-loadtester`
`image.pullPolicy` | Image pull policy | `IfNotPresent`
`image.tag` | Image tag | `<VERSION>`
`replicaCount` | Desired number of pods | `1`
`serviceAccountName` | Kubernetes service account name | `none` 
`resources.requests.cpu` | CPU requests | `10m`
`resources.requests.memory` | Memory requests | `64Mi`
`tolerations` | List of node taints to tolerate | `[]`
`affinity` | node/pod affinities | `node`
`nodeSelector` | Node labels for pod assignment | `{}`
`service.type` | Type of service | `ClusterIP`
`service.port` | ClusterIP port | `80`
`cmd.timeout` | Command execution timeout | `1h`
`logLevel` | Log level can be debug, info, warning, error or panic | `info`
`meshName` | AWS App Mesh name | `none`
`backends` | AWS App Mesh virtual services | `none`

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```console
helm install flagger/loadtester --name flagger-loadtester
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
helm install flagger/loadtester --name flagger-loadtester -f values.yaml
```

> **Tip**: You can use the default [values.yaml](values.yaml)


