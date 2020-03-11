# Frequently asked questions

### Deployment Strategies

**Which deployment strategies are supported by Flagger?**

Flagger implements the following deployment strategies:
* [Canary Release](usage/deployment-strategies.md#canary-release)
* [A/B Testing](usage/deployment-strategies.md#a-b-testing)
* [Blue/Green](usage/deployment-strategies.md#blue-green-deployments)
* [Blue/Green Mirroring](usage/deployment-strategies.md#blue-green-with-traffic-mirroring)

**When should I use A/B testing instead of progressive traffic shifting?**

For frontend applications that require session affinity you should use HTTP headers or cookies match conditions
to ensure a set of users will stay on the same version for the whole duration of the canary analysis.

**Can I use Flagger to manage applications that live outside of a service mesh?**

For applications that are not deployed on a service mesh, Flagger can orchestrate Blue/Green style deployments 
with Kubernetes L4 networking. 

**When can I use traffic mirroring?**

Traffic mirroring can be used for Blue/Green deployment strategy or a pre-stage in a Canary release.
Traffic mirroring will copy each incoming request, sending one request to the primary and one to the canary service.
Mirroring should be used for requests that are **idempotent** or capable of being processed twice (once by the primary and once by the canary).

**How to retry a failed release?**

A canary analysis is triggered by changes in any of the following objects:

* Deployment/DaemonSet PodSpec (metadata, container image, command, ports, env, resources, etc)
* ConfigMaps mounted as volumes or mapped to environment variables
* Secrets mounted as volumes or mapped to environment variables

To retry a release you can add or change an annotation on the pod template:

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    metadata:
      annotations:
        timestamp: "2020-03-10T14:24:48+0000"
```

### Kubernetes services

**How is an application exposed inside the cluster?**

Assuming the app name is podinfo you can define a canary like:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  service:
    # service name (optional)
    name: podinfo
    # ClusterIP port number (required)
    port: 9898
    # container port name or number
    targetPort: http
    # port name can be http or grpc (default http)
    portName: http
```

If the `service.name` is not specified, then `targetRef.name` is used for the apex domain and canary/primary services name prefix.
You should treat the service name as an immutable field, changing it could result in routing conflicts.

Based on the canary spec service, Flagger generates the following Kubernetes ClusterIP service:

* `<service.name>.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<service.name>-primary.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<service.name>-canary.<namespace>.svc.cluster.local`  
    selector `app=<name>`

This ensures that traffic coming from a namespace outside the mesh to `podinfo.test:9898`
will be routed to the latest stable release of your app. 

```yaml
apiVersion: v1
kind: Service
metadata:
  name: podinfo
spec:
  type: ClusterIP
  selector:
    app: podinfo-primary
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: http
---
apiVersion: v1
kind: Service
metadata:
  name: podinfo-primary
spec:
  type: ClusterIP
  selector:
    app: podinfo-primary
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: http
---
apiVersion: v1
kind: Service
metadata:
  name: podinfo-canary
spec:
  type: ClusterIP
  selector:
    app: podinfo
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: http
```

The `podinfo-canary.test:9898` address is available only during the 
canary analysis and can be used for conformance testing or load testing.

### Multiple ports

**My application listens on multiple ports, how can I expose them inside the cluster?**

If port discovery is enabled, Flagger scans the deployment spec and extracts the containers 
ports excluding the port specified in the canary service and Envoy sidecar ports. 
These ports will be used when generating the ClusterIP services.

For a deployment that exposes two ports:

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9899"
    spec:
      containers:
      - name: app
        ports:
        - containerPort: 8080
        - containerPort: 9090
```

You can enable port discovery so that Prometheus will be able to reach port `9090` over mTLS:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
spec:
  service:
    # container port used for canary analysis
    port: 8080
    # port name can be http or grpc (default http)
    portName: http
    # add all the other container ports
    # to the ClusterIP services (default false)
    portDiscovery: true
    trafficPolicy:
      tls:
        mode: ISTIO_MUTUAL
```

Both port `8080` and `9090` will be added to the ClusterIP services.

### Label selectors

**What labels selectors are supported by Flagger?**

The target deployment must have a single label selector in the format `app: <DEPLOYMENT-NAME>`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
spec:
  selector:
    matchLabels:
      app: podinfo
  template:
    metadata:
      labels:
        app: podinfo
```

Besides `app` Flagger supports `name` and `app.kubernetes.io/name` selectors. If you use a different 
convention you can specify your label with the `-selector-labels` flag.

**Is pod affinity and anti affinity supported?**

For pod affinity to work you need to use a different label than the `app`, `name` or `app.kubernetes.io/name`.

Anti affinity example:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
spec:
  selector:
    matchLabels:
      app: podinfo
      affinity: podinfo
  template:
    metadata:
      labels:
        app: podinfo
        affinity: podinfo
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  affinity: podinfo
              topologyKey: kubernetes.io/hostname
```

### Metrics

**How does Flagger measures the request success rate and duration?**

Flagger measures the request success rate and duration using Prometheus queries.

**HTTP requests success rate percentage**

Spec:

```yaml
  analysis:
    metrics:
    - name: request-success-rate
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      thresholdRange:
        min: 99
      interval: 1m
```

Istio query:

```javascript
sum(
    rate(
        istio_requests_total{
          reporter="destination",
          destination_workload_namespace=~"$namespace",
          destination_workload=~"$workload",
          response_code!~"5.*"
        }[$interval]
    )
) 
/ 
sum(
    rate(
        istio_requests_total{
          reporter="destination",
          destination_workload_namespace=~"$namespace",
          destination_workload=~"$workload"
        }[$interval]
    )
)
```

Envoy query (App Mesh, Contour or Gloo):

```javascript
sum(
    rate(
        envoy_cluster_upstream_rq{
          kubernetes_namespace="$namespace",
          kubernetes_pod_name=~"$workload",
          envoy_response_code!~"5.*"
        }[$interval]
    )
) 
/ 
sum(
    rate(
        envoy_cluster_upstream_rq{
          kubernetes_namespace="$namespace",
          kubernetes_pod_name=~"$workload"
        }[$interval]
    )
)
```

**HTTP requests milliseconds duration P99**

Spec:

```yaml
  analysis:
    metrics:
    - name: request-duration
      # maximum req duration P99
      # milliseconds
      thresholdRange:
        max: 500
      interval: 1m
```

Istio query:

```javascript
histogram_quantile(0.99, 
  sum(
    irate(
      istio_request_duration_seconds_bucket{
        reporter="destination",
        destination_workload=~"$workload",
        destination_workload_namespace=~"$namespace"
      }[$interval]
    )
  ) by (le)
)
```

Envoy query (App Mesh, Contour or Gloo):

```javascript
histogram_quantile(0.99, 
  sum(
    irate(
      envoy_cluster_upstream_rq_time_bucket{
        kubernetes_pod_name=~"$workload",
        kubernetes_namespace=~"$namespace"
      }[$interval]
    )
  ) by (le)
)
```

> **Note** that the metric interval should be lower or equal to the control loop interval.

**Can I use custom metrics?**

The analysis can be extended with metrics provided by Prometheus, Datadog and AWS CloudWatch. For more details 
on how custom metrics can be used please read the [metrics docs](usage/metrics.md).

### Istio routing

**How does Flagger interact with Istio?**

Flagger creates an Istio Virtual Service and Destination Rules based on the Canary service spec. 
The service configuration lets you expose an app inside or outside the mesh.
You can also define traffic policies, HTTP match conditions, URI rewrite rules, CORS policies, timeout and retries.

The following spec exposes the `frontend` workload inside the mesh on `frontend.test.svc.cluster.local:9898` 
and outside the mesh on `frontend.example.com`. You'll have to specify an Istio ingress gateway for external hosts.

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: frontend
  namespace: test
spec:
  service:
    # container port
    port: 9898
    # service port name (optional, will default to "http")
    portName: http-frontend
    # Istio gateways (optional)
    gateways:
    - public-gateway.istio-system.svc.cluster.local
    - mesh
    # Istio virtual service host names (optional)
    hosts:
    - frontend.example.com
    # Istio traffic policy
    trafficPolicy:
      tls:
        # use ISTIO_MUTUAL when mTLS is enabled
        mode: DISABLE
    # HTTP match conditions (optional)
    match:
      - uri:
          prefix: /
    # HTTP rewrite (optional)
    rewrite:
      uri: /
    # Istio retry policy (optional)
    retries:
      attempts: 3
      perTryTimeout: 1s
      retryOn: "gateway-error,connect-failure,refused-stream"
    # Add headers (optional)
    headers:
      request:
        add:
          x-some-header: "value"
    # cross-origin resource sharing policy (optional)
    corsPolicy:
      allowOrigin:
        - example.com
      allowMethods:
        - GET
      allowCredentials: false
      allowHeaders:
        - x-some-header
      maxAge: 24h
```

For the above spec Flagger will generate the following virtual service:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: frontend
  namespace: test
  ownerReferences:
    - apiVersion: flagger.app/v1beta1
      blockOwnerDeletion: true
      controller: true
      kind: Canary
      name: podinfo
      uid: 3a4a40dd-3875-11e9-8e1d-42010a9c0fd1
spec:
  gateways:
    - public-gateway.istio-system.svc.cluster.local
    - mesh
  hosts:
    - frontend.example.com
    - frontend
  http:
  - corsPolicy:
      allowHeaders:
      - x-some-header
      allowMethods:
      - GET
      allowOrigin:
      - example.com
      maxAge: 24h
    headers:
      request:
        add:
          x-some-header: "value"
    match:
    - uri:
        prefix: /
    rewrite:
      uri: /
    route:
    - destination:
        host: podinfo-primary
      weight: 100
    - destination:
        host: podinfo-canary
      weight: 0
    retries:
      attempts: 3
      perTryTimeout: 1s
      retryOn: "gateway-error,connect-failure,refused-stream"
```

For each destination in the virtual service a rule is generated:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: frontend-primary
  namespace: test
spec:
  host: frontend-primary
  trafficPolicy:
    tls:
      mode: DISABLE
---
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: frontend-canary
  namespace: test
spec:
  host: frontend-canary
  trafficPolicy:
    tls:
      mode: DISABLE
```

Flagger keeps in sync the virtual service and destination rules with the canary service spec.
Any direct modification to the virtual service spec will be overwritten.

To expose a workload inside the mesh on `http://backend.test.svc.cluster.local:9898`,
the service spec can contain only the container port and the traffic policy:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: backend
  namespace: test
spec:
  service:
    port: 9898
    trafficPolicy:
      tls:
        mode: DISABLE
```

Based on the above spec, Flagger will create several ClusterIP services like:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: backend-primary
  ownerReferences:
  - apiVersion: flagger.app/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: Canary
    name: backend
    uid: 2ca1a9c7-2ef6-11e9-bd01-42010a9c0145
spec:
  type: ClusterIP
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: 9898
  selector:
    app: backend-primary
```

Flagger works for user facing apps exposed outside the cluster via an ingress gateway
and for backend HTTP APIs that are accessible only from inside the mesh.

### Istio Ingress Gateway

**How can I expose multiple canaries on the same external domain?**

Assuming you have two apps, one that servers the main website and one that serves the REST API.
For each app you can define a canary object as:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: website
spec:
  service:
    port: 8080
    gateways:
    - public-gateway.istio-system.svc.cluster.local
    hosts:
    - my-site.com
    match:
      - uri:
          prefix: /
    rewrite:
      uri: /
---
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: webapi
spec:
  service:
    port: 8080
    gateways:
    - public-gateway.istio-system.svc.cluster.local
    hosts:
    - my-site.com
    match:
      - uri:
          prefix: /api
    rewrite:
      uri: /
```

Based on the above configuration, Flagger will create two virtual services bounded to the same ingress gateway and external host.
Istio Pilot will [merge](https://istio.io/help/ops/traffic-management/deploy-guidelines/#multiple-virtual-services-and-destination-rules-for-the-same-host)
the two services and the website rule will be moved to the end of the list in the merged configuration. 

Note that host merging only works if the canaries are bounded to a ingress gateway other than the `mesh` gateway.

### Istio Mutual TLS

**How can I enable mTLS for a canary?**

When deploying Istio with global mTLS enabled, you have to set the TLS mode to `ISTIO_MUTUAL`:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
spec:
  service:
    trafficPolicy:
      tls:
        mode: ISTIO_MUTUAL
```

If you run Istio in permissive mode you can disable TLS:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
spec:
  service:
    trafficPolicy:
      tls:
        mode: DISABLE
```

**If Flagger is outside of the mesh, how can it start the load test?**

In order for Flagger to be able to call the load tester service from outside the mesh, you need to disable mTLS on port 80:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: flagger-loadtester
  namespace: test
spec:
  host: "flagger-loadtester.test.svc.cluster.local"
  trafficPolicy:
    tls:
      mode: DISABLE
---
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: flagger-loadtester
  namespace: test
spec:
  targets:
  - name: flagger-loadtester
    ports:
    - number: 80
```
