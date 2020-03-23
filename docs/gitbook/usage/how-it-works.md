# How it works

[Flagger](https://github.com/weaveworks/flagger) can be configured to automate the release process 
for Kubernetes workloads with a custom resource named canary.

### Canary resource

The canary custom resource defines the release process of an application running on Kubernetes
and is portable across clusters, service meshes and ingress providers.

For a deployment named _podinfo_, a canary release with progressive traffic shifting can be defined as:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  service:
    port: 9898
  analysis:
    interval: 1m
    threshold: 10
    maxWeight: 50
    stepWeight: 5
    metrics:
      - name: request-success-rate
        thresholdRange:
          min: 99
        interval: 1m
      - name: request-duration
        thresholdRange:
          max: 500
        interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test:9898/"
```

When you deploy a new version of an app, Flagger gradually shifts traffic to the canary,
and at the same time, measures the requests success rate as well as the average response duration.
You can extend the canary analysis with custom metrics, acceptance and load testing
to harden the validation process of your app release process.

If you are running multiple service meshes or ingress controllers in the same cluster,
you can override the global provider for a specific canary with `spec.provider`.

### Canary target

A canary resource can target a Kubernetes Deployment or DaemonSet.

Kubernetes Deployment example:

```yaml
spec:
  progressDeadlineSeconds: 60
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  autoscalerRef:
    apiVersion: autoscaling/v2beta1
    kind: HorizontalPodAutoscaler
    name: podinfo
```

Based on the above configuration, Flagger generates the following Kubernetes objects:

* `deployment/<targetRef.name>-primary`
* `hpa/<autoscalerRef.name>-primary`

The primary deployment is considered the stable release of your app, by default all traffic is routed to this version 
and the target deployment is scaled to zero.
Flagger will detect changes to the target deployment (including secrets and configmaps) and will perform a
canary analysis before promoting the new version as primary.

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

The autoscaler reference is optional, when specified, Flagger will pause the traffic increase while the 
target and primary deployments are scaled up or down. HPA can help reduce the resource usage during the canary analysis.

The progress deadline represents the maximum time in seconds for the canary deployment to make progress
before it is rolled back, defaults to ten minutes.

### Canary service

A canary resource dictates how the target workload is exposed inside the cluster.
The canary target should expose a TCP port that will be used by Flagger to create the ClusterIP Services.

```yaml
spec:
  service:
    name: podinfo
    port: 9898
    portName: http
    targetPort: 9898
    portDiscovery: true
```

The container port from the target workload should match the `service.port` or `service.targetPort`.
The `service.name` is optional, defaults to `spec.targetRef.name`.
The `service.targetPort` can be a container port number or name.
The `service.portName` is optional (defaults to `http`), if your workload uses gRPC then set the port name to `grpc`.

If port discovery is enabled, Flagger scans the target workload and extracts the containers 
ports excluding the port specified in the canary service and service mesh sidecar ports. 
These ports will be used when generating the ClusterIP services.

Based on the canary spec service, Flagger creates the following Kubernetes ClusterIP service:

* `<service.name>.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<service.name>-primary.<namespace>.svc.cluster.local`  
    selector `app=<name>-primary`
* `<service.name>-canary.<namespace>.svc.cluster.local`  
    selector `app=<name>`

This ensures that traffic to `podinfo.test:9898` will be routed to the latest stable release of your app. 
The `podinfo-canary.test:9898` address is available only during the 
canary analysis and can be used for conformance testing or load testing.

Besides the port mapping, the service specification can contain URI match and rewrite rules,
timeout and retry polices:

```yaml
spec:
  service:
    port: 9898
    match:
      - uri:
          prefix: /
    rewrite:
      uri: /
    retries:
      attempts: 3
      perTryTimeout: 1s
    timeout: 5s
```

When using **Istio** as the mesh provider, you can also specify
HTTP header operations, CORS and traffic policies, Istio gateways and hosts.
The Istio routing configuration can be found [here](../faq.md#istio-routing).
 
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

### Canary finalizers

The default behavior of Flagger on canary deletion is to leave resources that aren't owned by the controller 
in their current state.  This simplifies the deletion action and avoids possible deadlocks during resource 
finalization.  In the event the canary was introduced with existing resource(s) (i.e. service, virtual service, etc.),
they would be mutated during the initialization phase and no longer reflect their initial state.  If the desired
functionality upon deletion is to revert the resources to their initial state, the `revertOnDeletion` attribute
can be enabled.  

```yaml
spec:
  revertOnDeletion: true
```

When a deletion action is submitted to the cluster, Flagger will attempt to revert the following resources:

* [Canary target](#canary-target) replicas will be updated to the primary replica count
* [Canary service](#canary-service) selector will be reverted
* Mesh/Ingress traffic routed to the target   

The recommended approach to disable canary analysis would be utilization of the `skipAnalysis`
attribute, which limits the need for resource reconciliation.  Utilizing the `revertOnDeletion` attribute should be 
enabled when you no longer plan to rely on Flagger for deployment management.

**Note** When this feature is enabled expect a delay in the delete action due to the reconciliation.  

### Canary analysis

The canary analysis defines:
* the type of [deployment strategy](deployment-strategies.md)
* the [metrics](metrics.md) used to validate the canary version
* the [webhooks](webhooks.md) used for conformance testing, load testing and manual gating
* the [alerting settings](alerting.md)

Spec:

```yaml
  analysis:
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
