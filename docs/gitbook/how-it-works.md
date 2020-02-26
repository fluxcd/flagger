# How it works

[Flagger](https://github.com/weaveworks/flagger) takes a Kubernetes deployment and optionally 
a horizontal pod autoscaler (HPA) and creates a series of objects 
(Kubernetes deployments, ClusterIP services, virtual service, traffic split or ingress)
to drive the canary analysis and promotion.

### Canary Custom Resource

For a deployment named _podinfo_, a canary promotion can be defined using Flagger's custom resource:

```yaml
apiVersion: flagger.app/v1alpha3
kind: Canary
metadata:
  name: podinfo
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  autoscalerRef:
    apiVersion: autoscaling/v2beta1
    kind: HorizontalPodAutoscaler
    name: podinfo
  service:
    name: podinfo
    port: 9898
    portName: http
    targetPort: 9898
    portDiscovery: true
  canaryAnalysis:
    interval: 1m
    threshold: 10
    maxWeight: 50
    stepWeight: 5
    metrics:
      - name: request-success-rate
        threshold: 99
        interval: 1m
      - name: request-duration
        threshold: 99
        interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test:9898/"
```

Based on the above configuration, Flagger generates the following Kubernetes objects:
* `deployment/<targetRef.name>-primary`
* `hpa/<autoscalerRef.name>-primary`

The primary deployment is considered the stable release of your app, by default all traffic is routed to this version 
and the target deployment is scaled to zero.
Flagger will detect changes to the target deployment (including secrets and configmaps) and will perform a
canary analysis before promoting the new version as primary.

The autoscaler reference is optional, when specified, Flagger will pause the traffic increase while the 
target and primary deployments are scaled up or down. HPA can help reduce the resource usage during the canary analysis.

If the target deployment uses secrets and/or configmaps, Flagger will create a copy of each object using the `-primary`
prefix and will reference these objects in the primary deployment. You can disable the secrets/configmaps tracking 
with the `-enable-config-tracking=false` command flag in the Flagger deployment manifest under containers args
or by setting `--set configTracking.enabled=false` when installing Flagger with Helm.

**Note** that the target deployment must have a single label selector in the format `app: <DEPLOYMENT-NAME>`:

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

Besides `app` Flagger supports `name` and `app.kubernetes.io/name` selectors.
If you use a different convention you can specify your label with
the `-selector-labels=my-app-label` command flag in the Flagger deployment manifest under containers args
or by setting `--set selectorLabels=my-app-label` when installing Flagger with Helm.

The target deployment should expose a TCP port that will be used by Flagger to create the ClusterIP Services.
The container port from the target deployment should match the `service.port` or `service.targetPort`.

Based on the canary spec service, Flagger generates the following Kubernetes ClusterIP service:

* `<service.name>.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<service.name>-primary.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<service.name>-canary.<namespace>.svc.cluster.local`  
    selector `app=<name>`

This ensures that traffic to `podinfo.test:9898` will be routed to the latest stable release of your app. 
The `podinfo-canary.test:9898` address is available only during the 
canary analysis and can be used for conformance testing or load testing.

### Canary status

You can use kubectl to get the current status of canary deployments cluster wide: 

```bash
kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   15       2019-06-30T14:05:07Z
prod        frontend  Succeeded     0        2019-06-30T16:15:07Z
prod        backend   Failed        0        2019-06-30T17:05:07Z
```

The status condition reflects the last known state of the canary analysis:

```bash
kubectl -n test get canary/podinfo -oyaml | awk '/status/,0'
```

A successful rollout status:

```yaml
status:
  canaryWeight: 0
  failedChecks: 0
  iterations: 0
  lastAppliedSpec: "14788816656920327485"
  lastPromotedSpec: "14788816656920327485"
  conditions:
  - lastTransitionTime: "2019-07-10T08:23:18Z"
    lastUpdateTime: "2019-07-10T08:23:18Z"
    message: Canary analysis completed successfully, promotion finished.
    reason: Succeeded
    status: "True"
    type: Promoted
```

The `Promoted` status condition can have one of the following reasons:
Initialized, Waiting, Progressing, Promoting, Finalising, Succeeded or Failed.
A failed canary will have the promoted status set to `false`,
the reason to `failed` and the last applied spec will be different to the last promoted one.

Wait for a successful rollout:

```bash
kubectl wait canary/podinfo --for=condition=promoted
```

CI example:

```bash
# update the container image
kubectl set image deployment/podinfo podinfod=stefanprodan/podinfo:3.0.1

# wait for Flagger to detect the change
ok=false
until ${ok}; do
    kubectl get canary/podinfo | grep 'Progressing' && ok=true || ok=false
    sleep 5
done

# wait for the canary analysis to finish
kubectl wait canary/podinfo --for=condition=promoted --timeout=5m

# check if the deployment was successful 
kubectl get canary/podinfo | grep Succeeded
```

### Canary Analysis

The canary analysis defines:
* the type of [deployment strategy](usage/deployment-strategies.md)
* the [metrics](usage/metrics.md) used to validate the canary version
* the [webhooks](usage/webhooks.md) used for conformance testing, load testing and manual gating
* the [alerting settings](usage/alerting.md)

Spec:

```yaml
  canaryAnalysis:
    # schedule interval (default 60s)
    interval:
    # max number of failed metric checks before rollback
    threshold:
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight:
    # canary increment step
    # percentage (0-100)
    stepWeight:
    # total number of iterations
    # used for A/B Testing and Blue/Green
    iterations:
    # canary match conditions
    # used for A/B Testing
    match:
      - # HTTP header
    # key performance indicators
    metrics:
      - # metric check
    # alerting
    alerts:
      - # alert provider
    # external checks
    webhooks:
      - # hook
```

The canary analysis runs periodically until it reaches the maximum traffic weight or the number of iterations.
On each run, Flagger calls the webhooks, checks the metrics and if the failed checks threshold is reached, stops the
analysis and rolls back the canary. If alerting is configured, Flagger will post the analysis result using the alert providers.

