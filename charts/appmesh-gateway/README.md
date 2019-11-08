# Flagger Gateway for App Mesh

[Flagger Gateway for App Mesh](https://github.com/stefanprodan/appmesh-gateway) is an
Envoy-powered load balancer that exposes applications outside the mesh.
The gateway facilitates canary deployments and A/B testing for user-facing web applications running on AWS App Mesh.

## Prerequisites

* Kubernetes >= 1.13
* [App Mesh controller](https://github.com/aws/eks-charts/tree/master/stable/appmesh-controller) >= 0.2.0
* [App Mesh inject](https://github.com/aws/eks-charts/tree/master/stable/appmesh-inject) >= 0.2.0

## Installing the Chart

Add Flagger Helm repository:

```console
$ helm repo add flagger https://flagger.app
```

Create a namespace with App Mesh sidecar injection enabled:

```sh
kubectl create ns flagger-system
kubectl label namespace test appmesh.k8s.aws/sidecarInjectorWebhook=enabled
```

Install App Mesh Gateway for an existing mesh:

```sh
helm upgrade -i appmesh-gateway flagger/appmesh-gateway \
--namespace flagger-system \
--set mesh.name=global
```

Optionally you can create a mesh at install time:
  
```sh
helm upgrade -i appmesh-gateway flagger/appmesh-gateway \
--namespace flagger-system \
--set mesh.name=global \
--set mesh.create=true
```

The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `appmesh-gateway` deployment:

```console
helm delete --purge appmesh-gateway
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the chart and their default values.

Parameter | Description | Default
--- | --- | ---
`service.type` |  When set to LoadBalancer it creates an AWS NLB | `LoadBalancer`
`proxy.access_log_path` | to enable the access logs, set it to `/dev/stdout` | `/dev/null`
`proxy.image.repository` | image repository | `envoyproxy/envoy`
`proxy.image.tag` | image tag | `<VERSION>`
`proxy.image.pullPolicy` | image pull policy | `IfNotPresent`
`controller.image.repository` | image repository | `weaveworks/flagger-appmesh-gateway`
`controller.image.tag` | image tag | `<VERSION>`
`controller.image.pullPolicy` | image pull policy | `IfNotPresent`
`resources.requests/cpu` | pod CPU request | `100m`
`resources.requests/memory` | pod memory request | `128Mi`
`resources.limits/memory` | pod memory limit | `2Gi`
`nodeSelector` | node labels for pod assignment | `{}`
`tolerations` | list of node taints to tolerate | `[]`
`rbac.create` | if `true`, create and use RBAC resources | `true`
`rbac.pspEnabled` | If `true`, create and use a restricted pod security policy | `false`
`serviceAccount.create` | If `true`, create a new service account | `true`
`serviceAccount.name` | Service account to be used | None
`mesh.create` | If `true`, create mesh custom resource | `false`
`mesh.name` | The name of the mesh to use | `global`
`mesh.discovery` | The service discovery type to use, can be dns or cloudmap | `dns`
`hpa.enabled` | `true` if HPA resource should be created, metrics-server is required | `true`
`hpa.maxReplicas` | number of max replicas | `3`
`hpa.cpu` |  average total CPU usage per pod (1-100) | `99`
`hpa.memory` |  average memory usage per pod (100Mi-1Gi) | None
`discovery.optIn` | `true` if only services with the 'expose' annotation are discoverable | `true`
