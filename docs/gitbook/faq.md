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
