# Frequently asked questions

### Deployment Strategies

**Which deployment strategies are supported by Flagger?**

Flagger can run automated application analysis, promotion and rollback for the following deployment strategies:
* Canary (progressive traffic shifting)
    * Istio, Linkerd, App Mesh, NGINX, Gloo
* Canary (traffic mirroring)
    * Istio
* A/B Testing (HTTP headers and cookies traffic routing)
    * Istio, NGINX
* Blue/Green (traffic switch)
    * Kubernetes CNI, Istio, Linkerd, App Mesh, NGINX, Gloo

For Canary deployments and A/B testing you'll need a Layer 7 traffic management solution like a service mesh or an ingress controller.
For Blue/Green deployments no service mesh or ingress controller is required.

**When should I use A/B testing instead of progressive traffic shifting?**

For frontend applications that require session affinity you should use HTTP headers or cookies match conditions
to ensure a set of users will stay on the same version for the whole duration of the canary analysis.
A/B testing is supported by Istio and NGINX only.

Istio example:

```yaml
  canaryAnalysis:
    # schedule interval (default 60s)
    interval: 1m
    # total number of iterations
    iterations: 10
    # max number of failed iterations before rollback
    threshold: 2
    # canary match condition
    match:
      - headers:
          x-canary:
            regex: ".*insider.*"
      - headers:
          cookie:
            regex: "^(.*?;)?(canary=always)(;.*)?$"
```

NGINX example:

```yaml
  canaryAnalysis:
    interval: 1m
    threshold: 10
    iterations: 2
    match:
      - headers:
          x-canary:
            exact: "insider"
      - headers:
          cookie:
            exact: "canary"
```

Note that the NGINX ingress controller supports only exact matching for a single header and the cookie value is set to `always`.

The above configurations will route users with the x-canary header or canary cookie to the canary instance during analysis:

```bash
curl -H 'X-Canary: insider' http://app.example.com
curl -b 'canary=always' http://app.example.com
```

**Can I use Flagger to manage applications that live outside of a service mesh?**

For applications that are not deployed on a service mesh, Flagger can orchestrate Blue/Green style deployments 
with Kubernetes L4 networking. 

Blue/Green example:

```yaml
apiVersion: flagger.app/v1alpha3
kind: Canary
spec:
  provider: kubernetes
  canaryAnalysis:
    interval: 30s
    threshold: 2
    iterations: 10       
    metrics:
      - name: request-success-rate
        threshold: 99
        interval: 1m
      - name: request-duration
        threshold: 500
        interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test:9898/" 
```

The above configuration will run an analysis for five minutes. 
Flagger starts the load test for the canary service (green version) and checks the Prometheus metrics every 30 seconds.
If the analysis result is positive, Flagger will promote the canary (green version) to primary (blue version).

**When can I use traffic mirroring?**

Traffic Mirroring is a pre-stage in a Canary (progressive traffic shifting) or
Blue/Green deployment strategy. Traffic mirroring will copy each incoming
request, sending one request to the primary and one to the canary service.
The response from the primary is sent back to the user. The response from the canary
is discarded.  Metrics are collected on both requests so that the deployment will
only proceed if the canary metrics are healthy.

Mirroring is supported by Istio only.

In Istio, mirrored requests have `-shadow` appended to the `Host` (HTTP) or
`Authority` (HTTP/2) header; for example requests to `podinfo.test` that are
mirrored will be reported in telemetry with a destination host `podinfo.test-shadow`.

Mirroring must only be used for requests that are **idempotent** or capable of
being processed twice (once by the primary and once by the canary). Reads are
idempotent. Before using mirroring on requests that may be writes, you should
consider what will happen if a write is duplicated and handled by the primary
and canary.

To use mirroring, set `spec.canaryAnalysis.mirror` to `true`. Example for
traffic shifting:

```yaml
apiVersion: flagger.app/v1alpha3
kind: Canary
spec:
  provider: istio
  canaryAnalysis:
    mirror: true
    interval: 30s
    stepWeight: 20
    maxWeight: 50
```

### Kubernetes services

**How is an application exposed inside the cluster?**

Assuming the app name is podinfo you can define a canary like:

```yaml
apiVersion: flagger.app/v1alpha3
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
    # ClusterIP port number (required)
    port: 9898
    # container port name or number
    targetPort: http
    # port name can be http or grpc (default http)
    portName: http
```

Based on the canary spec service, Flagger generates the following Kubernetes ClusterIP service:

* `<targetRef.name>.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<targetRef.name>-primary.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<targetRef.name>-canary.<namespace>.svc.cluster.local`  
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
`These ports will be used when generating the ClusterIP services.

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
apiVersion: flagger.app/v1alpha3
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

### Istio routing

**How does Flagger interact with Istio?**

Flagger creates an Istio Virtual Service and Destination Rules based on the Canary service spec. 
The service configuration lets you expose an app inside or outside the mesh.
You can also define traffic policies, HTTP match conditions, URI rewrite rules, CORS policies, timeout and retries.

The following spec exposes the `frontend` workload inside the mesh on `frontend.test.svc.cluster.local:9898` 
and outside the mesh on `frontend.example.com`. You'll have to specify an Istio ingress gateway for external hosts.

```yaml
apiVersion: flagger.app/v1alpha3
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
    - apiVersion: flagger.app/v1alpha3
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
  - appendHeaders:
      x-some-header: "value"
    corsPolicy:
      allowHeaders:
      - x-some-header
      allowMethods:
      - GET
      allowOrigin:
      - example.com
      maxAge: 24h
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
apiVersion: flagger.app/v1alpha3
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
  - apiVersion: flagger.app/v1alpha3
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
apiVersion: flagger.app/v1alpha3
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
apiVersion: flagger.app/v1alpha3
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
apiVersion: flagger.app/v1alpha3
kind: Canary
spec:
  service:
    trafficPolicy:
      tls:
        mode: ISTIO_MUTUAL
```

If you run Istio in permissive mode you can disable TLS:

```yaml
apiVersion: flagger.app/v1alpha3
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
