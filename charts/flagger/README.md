# Flagger

Flagger is a Kubernetes operator that automates the promotion of canary deployments
using Istio routing for traffic shifting and Prometheus metrics for canary analysis.

## Installing the Chart

To install the chart with the release name `flagger`:

```console
$ helm upgrade --install flagger ./charts/flagger --namespace=istio-system
```

The command deploys Flagger on the Kubernetes cluster in the istio-system namespace.
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
`image.repository` | image repository | `stefanprodan/flagger`
`image.tag` | image tag | `<VERSION>`
`image.pullPolicy` | image pull policy | `IfNotPresent`
`resources.requests/cpu` | pod CPU request | `10m`
`resources.requests/memory` | pod memory request | `32Mi`
`resources.limits/cpu` | pod CPU limit | `1000m`
`resources.limits/memory` | pod memory limit | `512Mi`
`affinity` | node/pod affinities | None
`nodeSelector` | node labels for pod assignment | `{}`
`tolerations` | list of node taints to tolerate | `[]`

Specify each parameter using the `--set key=value[,key=value]` argument to `helm upgrade`. For example,

```console
$ helm upgrade --install flagger ./charts/flagger \
  --namespace=istio-system \
  --set=image.tag=0.0.2
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm upgrade --install flagger ./charts/flagger \
  --namespace=istio-system \
  -f values.yaml
```

> **Tip**: You can use the default [values.yaml](values.yaml)
```

