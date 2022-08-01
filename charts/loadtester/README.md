# Flagger load testing service

[Flagger's](https://github.com/fluxcd/flagger) load testing service is based on
[rakyll/hey](https://github.com/rakyll/hey) and
[bojand/ghz](https://github.com/bojand/ghz).
It can be used to generate HTTP and gRPC traffic during canary analysis when configured as a webhook.

## Prerequisites

* Kubernetes >= 1.19

## Installing the Chart

Add Flagger Helm repository:

```console
helm repo add flagger https://flagger.app
```

To install the chart with the release name `flagger-loadtester`:

```console
helm upgrade -i flagger-loadtester flagger/loadtester
```

The command deploys loadtester on the Kubernetes cluster in the default namespace.

> **Tip**: Note that the namespace where you deploy the load tester should
> have the Istio, App Mesh, Linkerd or Open Service Mesh sidecar injection enabled

The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `flagger-loadtester` deployment:

```console
helm delete flagger-loadtester
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the load tester chart and their default values.

| Parameter                          | Description                                                                          | Default                             |
|------------------------------------|--------------------------------------------------------------------------------------|-------------------------------------|
| `image.repository`                 | Image repository                                                                     | `ghcr.io/fluxcd/flagger-loadtester` |
| `image.pullPolicy`                 | Image pull policy                                                                    | `IfNotPresent`                      |
| `image.tag`                        | Image tag                                                                            | `<VERSION>`                         |
| `replicaCount`                     | Desired number of pods                                                               | `1`                                 |
| `serviceAccountName`               | Kubernetes service account name                                                      | `none`                              |
| `resources.requests.cpu`           | CPU requests                                                                         | `10m`                               |
| `resources.requests.memory`        | Memory requests                                                                      | `64Mi`                              |
| `tolerations`                      | List of node taints to tolerate                                                      | `[]`                                |
| `affinity`                         | node/pod affinities                                                                  | `node`                              |
| `nodeSelector`                     | Node labels for pod assignment                                                       | `{}`                                |
| `service.type`                     | Type of service                                                                      | `ClusterIP`                         |
| `service.port`                     | ClusterIP port                                                                       | `80`                                |
| `cmd.timeout`                      | Command execution timeout                                                            | `1h`                                |
| `cmd.namespaceRegexp`              | Restrict access to canaries in matching namespaces                                   | ""                                  |
| `logLevel`                         | Log level can be debug, info, warning, error or panic                                | `info`                              |
| `appmesh.enabled`                  | Create AWS App Mesh v1beta2 virtual node                                             | `false`                             |
| `appmesh.backends`                 | AWS App Mesh virtual services                                                        | `none`                              |
| `istio.enabled`                    | Create Istio virtual service                                                         | `false`                             |
| `istio.host`                       | Loadtester hostname                                                                  | `flagger-loadtester.flagger`        |
| `istio.gateway.enabled`            | Create Istio gateway in namespace                                                    | `false`                             |
| `istio.tls.enabled`                | Enable TLS in gateway ( TLS secrets should be in namespace )                         | `false`                             |
| `istio.tls.httpsRedirect`          | Redirect traffic to TLS port                                                         | `false`                             |
| `podPriorityClassName`             | PriorityClass name for pod priority configuration                                    | ""                                  |
| `securityContext.enabled`          | Add securityContext to container                                                     | ""                                  |
| `securityContext.context`          | securityContext to add                                                               | ""                                  |
| `podDisruptionBudget.enabled`      | A PodDisruptionBudget will be created if `true`                                      | `false`                             |
| `podDisruptionBudget.minAvailable` | The minimal number of available replicas that will be set in the PodDisruptionBudget | `1`                                 |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm upgrade`. For example,

```console
helm upgrade -i flagger-loadtester flagger/loadtester \
--set "appmesh.enabled=true" \
--set "appmesh.backends[0]=podinfo" \
--set "appmesh.backends[1]=podinfo-canary"
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
helm install flagger/loadtester --name flagger-loadtester -f values.yaml
```

> **Tip**: You can use the default [values.yaml](values.yaml)
