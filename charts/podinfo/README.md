# Podinfo

Podinfo is a tiny web application made with Go 
that showcases best practices of running canary deployments with Flagger and Istio.

## Installing the Chart

Add Flagger Helm repository:

```console
helm repo add flagger https://flagger.app
```

To install the chart with the release name `frontend`:

```console
helm upgrade -i frontend flagger/podinfo \
--namespace test \
--set nameOverride=frontend \
--set backend=http://backend.test:9898/echo \
--set canary.enabled=true \
--set canary.istioIngress.enabled=true \
--set canary.istioIngress.gateway=istio-system/public-gateway \
--set canary.istioIngress.host=frontend.istio.example.com
```

To install the chart as `backend`:

```console
helm upgrade -i backend flagger/podinfo \
--namespace test \
--set nameOverride=backend \
--set canary.enabled=true 
```

## Uninstalling the Chart

To uninstall/delete the `frontend` deployment:

```console
$ helm delete --purge frontend
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the podinfo chart and their default values.

Parameter | Description | Default
--- | --- | ---
`image.repository` | image repository | `quay.io/stefanprodan/podinfo`
`image.tag` | image tag | `<VERSION>`
`image.pullPolicy` | image pull policy | `IfNotPresent`
`hpa.enabled` | enables HPA | `true`
`hpa.cpu` | target CPU usage per pod | `80`
`hpa.memory` | target memory usage per pod | `512Mi`
`hpa.minReplicas` | maximum pod replicas | `2`
`hpa.maxReplicas` | maximum pod replicas | `4`
`resources.requests/cpu` | pod CPU request | `1m`
`resources.requests/memory` | pod memory request | `16Mi`
`backend` | backend URL | None
`faults.delay` | random HTTP response delays between 0 and 5 seconds | `false`
`faults.error` | 1/3 chances of a random HTTP response error | `false`

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```console
$ helm install flagger/podinfo --name frontend \
  --set=image.tag=1.4.1,hpa.enabled=false
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm install flagger/podinfo --name frontend -f values.yaml
```


