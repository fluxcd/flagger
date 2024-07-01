# Canary analysis with KEDA ScaledObjects

This guide shows you how to use Flagger with KEDA ScaledObjects to autoscale workloads during a Canary analysis run.
We will be using a Blue/Green deployment strategy with the Kubernetes provider for the sake of this tutorial, but
you can use any deployment strategy combined with any supported provider.

## Prerequisites

Flagger requires a Kubernetes cluster **v1.16** or newer. For this tutorial, we'll need KEDA **2.7.1** or newer.

Install KEDA:

```bash
helm repo add kedacore https://kedacore.github.io/charts
kubectl create namespace keda
helm install keda kedacore/keda --namespace keda
```

Install Flagger:
```bash
helm repo add flagger https://flagger.app

kubectl create namespace flagger
helm upgrade -i flagger flagger/flagger \
--namespace flagger \
--set prometheus.install=true \
--set meshProvider=kubernetes
```

## Bootstrap

Flagger takes a Kubernetes deployment and a KEDA ScaledObject targeting the deployment. It then creates a series of objects 
(Kubernetes deployments, ClusterIP services and another KEDA ScaledObject targeting the created Deployment).
These objects expose the application inside the mesh and drive the Canary analysis and Blue/Green promotion.

Create a test namespace:

```bash
kubectl create ns test
```

Create a deployment named `podinfo`:

```bash
kubectl apply -n test -f https://raw.githubusercontent.com/fluxcd/flagger/main/kustomize/podinfo/deployment.yaml
```

Deploy the load testing service to generate traffic during the analysis:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/tester?ref=main
```

Create a ScaledObject which targets the `podinfo` deployment and uses Prometheus as a trigger:
```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: podinfo-so
  namespace: test
spec:
  scaleTargetRef:
    name: podinfo
  pollingInterval: 10
  cooldownPeriod: 20
  minReplicaCount: 1
  maxReplicaCount: 3
  triggers:
  - type: prometheus
    metadata:
      name: prom-trigger
      serverAddress: http://flagger-prometheus.flagger:9090
      metricName: http_requests_total
      query: sum(rate(http_requests_total{ app="podinfo" }[30s]))
      threshold: '5'
```

Create a canary custom resource for the `podinfo` deployment:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: kubernetes
  # deployment reference
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  # Scaler reference
  autoscalerRef:
    apiVersion: keda.sh/v1alpha1
    kind: ScaledObject
    # ScaledObject targeting the canary deployment
    name: podinfo-so
    # Mapping between trigger names and the related query to use for the generated 
    # ScaledObject targeting the primary deployment. (Optional)
    primaryScalerQueries:
      prom-trigger: sum(rate(http_requests_total{ app="podinfo-primary" }[30s]))
    # Overriding replica scaling configuration for the generated ScaledObject
    # targeting the primary deployment. (Optional)
    primaryScalerReplicas:
      minReplicas: 2
      maxReplicas: 5
  # the maximum time in seconds for the canary deployment
  # to make progress before rollback (default 600s)
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
    name: podinfo-svc
    portDiscovery: true
  analysis:
    # schedule interval (default 60s)
    interval: 15s
    # max number of failed checks before rollback
    threshold: 5
    # number of checks to run before promotion
    iterations: 5
    # Prometheus checks based on 
    # http_request_duration_seconds histogram
    metrics:
      - name: request-success-rate
        interval: 1m
        thresholdRange:
          min: 99
      - name: request-duration
        interval: 30s
        thresholdRange:
          max: 500
    # load testing hooks
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 20 -c 2 http://podinfo-svc-canary.test/"
```

Save the above resource as `podinfo-canary.yaml` and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

After a couple of seconds Flagger will create the canary objects:

```bash
# applied
deployment.apps/podinfo
scaledobject.keda.sh/podinfo-so
canary.flagger.app/podinfo

# generated
deployment.apps/podinfo-primary
horizontalpodautoscaler.autoscaling/podinfo-primary
service/podinfo
service/podinfo-canary
service/podinfo-primary
scaledobject.keda.sh/podinfo-so-primary
```

We refer to our ScaledObject for the canary deployment using `.spec.autoscalerRef`. Flagger will use this to generate a ScaledObject which will scale the primary deployment.
By default, Flagger will try to guess the query to use for the primary ScaledObject, by replacing all mentions of `.spec.targetRef.Name` and `{.spec.targetRef.Name}-canary`
with `{.spec.targetRef.Name}-primary`, for all triggers.
For eg, if your ScaledObject has a trigger query defined as: `sum(rate(http_requests_total{ app="podinfo" }[30s]))` or `sum(rate(http_requests_total{ app="podinfo-primary" }[30s]))`, then the primary ScaledObject will have the same trigger with a query defined as `sum(rate(http_requests_total{ app="podinfo-primary" }[30s]))`.

If, the generated query does not meet your requirements, you can specify the query for autoscaling the primary deployment explicitly using 
`.spec.autoscalerRef.primaryScalerQueries`, which lets you define a query for each trigger. Please note that, your ScaledObject's `.spec.triggers[@].name` must
not be blank, as Flagger needs that to identify each trigger uniquely.

In the situation when it is desired to have different scaling replica configuration between the canary and primary deployment ScaledObject you can use
the `.spec.autoscalerRef.primaryScalerReplicas` to override these values for the generated primary ScaledObject.

After the boostrap, the podinfo deployment will be scaled to zero and the traffic to `podinfo.test` will be routed to the primary pods. To keep the podinfo deployment
at 0 replicas and pause auto scaling, Flagger will add an annotation to your ScaledObject: `autoscaling.keda.sh/paused-replicas: 0`.
During the canary analysis, the annotation is removed, to enable auto scaling for the podinfo deployment.
The `podinfo-canary.test` address can be used to target directly the canary pods. 
When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary. The Blue/Green deployment will run for five iterations while validating the HTTP metrics and rollout hooks every 15 seconds.


## Automated Blue/Green promotion

Trigger a deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.1
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/podinfo

Events:

New revision detected podinfo.test
Waiting for podinfo.test rollout to finish: 0 of 1 updated replicas are available
Pre-rollout check acceptance-test passed
Advance podinfo.test canary iteration 1/10
Advance podinfo.test canary iteration 2/10
Advance podinfo.test canary iteration 3/10
Advance podinfo.test canary iteration 4/10
Advance podinfo.test canary iteration 5/10
Advance podinfo.test canary iteration 6/10
Advance podinfo.test canary iteration 7/10
Advance podinfo.test canary iteration 8/10
Advance podinfo.test canary iteration 9/10
Advance podinfo.test canary iteration 10/10
Copying podinfo.test template spec to podinfo-primary.test
Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
Promotion completed! Scaling down podinfo.test
```

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   100      2019-06-16T14:05:07Z
```

You can monitor the scaling of the deployments with:
```bash
watch kubectl -n test get deploy podinfo
NAME                 READY   UP-TO-DATE   AVAILABLE   AGE
flagger-loadtester   1/1     1            1           4m21s
podinfo              3/3     3            3           4m28s
podinfo-primary      3/3     3            3           3m14s
```

You can mointor how Flagger edits the annotations of your ScaledObject with:
```bash
watch "kubectl get -n test scaledobjects podinfo-so -o=jsonpath='{.metadata.annotations}'"
```
