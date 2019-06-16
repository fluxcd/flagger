# Frequently asked questions

### A/B Testing

When should I use A/B testing instead of progressive traffic shifting?

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

### Kubernetes services

How is an application exposed inside the cluster?

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
    # container port (required)
    port: 9898
    # port name can be http or grpc (default http)
    portName: http
```

Based on the canary spec service, Flagger generates the following Kubernetes ClusterIP service:

* `<name>.<namespaces>.vc.cluster.local` with selector `app=<name>-primary`
* `<name>-primary.<namespaces>.vc.cluster.local` with selector `app=<name>-primary`
* `<name>-canary.<namespaces>.vc.cluster.local` with selector `app=<name>`

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
```

The `podinfo-canary.test:9898` address is available only during the 
canary analysis and can be used for conformance testing or load testing.

### Multiple ports

My application listens on multiple ports, how can I expose them inside the cluster?

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

Both port `8080` and `9090` will be added to the ClusterIP services but the virtual service
will point to the port specified in `spec.service.port`.

### Label selectors

What labels selectors are supported by Flagger?

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

Is pod affinity and anti affinity supported?

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

### Istio Mutual TLS

How can I enable mTLS for a canary?

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

If Flagger is outside of the mesh, how can it start the load test?

In order for Flagger to be able to call the load tester service from outside the mesh, you need to disable mTLS on port 80:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: flagger-namespace
  namespace: flagger
spec:
  host: "*.flagger.svc.cluster.local"
  trafficPolicy:
    tls:
      mode: DISABLE
---
apiVersion: authentication.istio.io/v1alpha1
kind: Policy
metadata:
  name: loadtester-mtls-disabled
  namespace: flagger
spec:
  targets:
  - name: flagger-loadtester
    ports:
    - number: 80
```
