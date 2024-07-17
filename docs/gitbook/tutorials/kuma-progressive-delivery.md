# Kuma Canary Deployments

This guide shows you how to use Kuma and Flagger to automate canary deployments.

![Flagger Kuma Canary](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-kuma-canary.png)

## Prerequisites

Flagger requires a Kubernetes cluster **v1.19** or newer and Kuma **1.7** or newer.

Install Kuma and Prometheus (part of Kuma Metrics):

```bash
kumactl install control-plane | kubectl apply -f -
kumactl install observability --components "grafana,prometheus" | kubectl apply -f -
```

Install Flagger in the `kong-mesh-system` namespace:

```bash
kubectl apply -k github.com/fluxcd/flagger//kustomize/kuma
```

## Bootstrap

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services and Kuma `TrafficRoute`).
These objects expose the application inside the mesh and drive the canary analysis and promotion.

Create a test namespace and enable Kuma sidecar injection:

```bash
kubectl create ns test
kubectl annotate namespace test kuma.io/sidecar-injection=enabled
```

Install the load testing service to generate traffic during the canary analysis:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/tester?ref=main
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/podinfo?ref=main
```

Create a canary custom resource for the `podinfo` deployment:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
  annotations:
    kuma.io/mesh: default
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    targetPort: 9898
    apex:
      annotations:
        9898.service.kuma.io/protocol: "http"
    canary:
      annotations:
        9898.service.kuma.io/protocol: "http"
    primary:
      annotations:
        9898.service.kuma.io/protocol: "http"
  analysis:
    # schedule interval (default 60s)
    interval: 30s
    # max number of failed metric checks before rollback
    threshold: 5
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 5
    metrics:
      - name: request-success-rate
        threshold: 99
        interval: 1m
      - name: request-duration
        threshold: 500
        interval: 30s
    webhooks:
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 http://podinfo-canary.test:9898/"
```

Save the above resource as `podinfo-canary.yaml` and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary. The canary analysis will run for five minutes while validating the HTTP metrics and rollout hooks every half a minute.

After a couple of seconds Flagger will create the canary objects:

```bash
# applied
deployment.apps/podinfo
horizontalpodautoscaler.autoscaling/podinfo
ingresses.extensions/podinfo
canary.flagger.app/podinfo

# generated
deployment.apps/podinfo-primary
horizontalpodautoscaler.autoscaling/podinfo-primary
service/podinfo
service/podinfo-canary
service/podinfo-primary
trafficroutes.kuma.io/podinfo
```

After the bootstrap, the podinfo deployment will be scaled to zero and the traffic to `podinfo.test` will be routed to the primary pods. During the canary analysis, the `podinfo-canary.test` address can be used to target directly the canary pods.

## Automated canary promotion

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance indicators like HTTP requests success rate, requests average duration and pod health. Based on analysis of the KPIs a canary is promoted or aborted, and the analysis result is published to Slack.

![Flagger Canary Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-canary-steps.png)

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.1
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:         0
  Failed Checks:         0
  Phase:                 Succeeded
Events:
 New revision detected! Scaling up podinfo.test
 Waiting for podinfo.test rollout to finish: 0 of 1 updated replicas are available
 Pre-rollout check acceptance-test passed
 Advance podinfo.test canary weight 5
 Advance podinfo.test canary weight 10
 Advance podinfo.test canary weight 15
 Advance podinfo.test canary weight 20
 Advance podinfo.test canary weight 25
 Waiting for podinfo.test rollout to finish: 1 of 2 updated replicas are available
 Advance podinfo.test canary weight 30
 Advance podinfo.test canary weight 35
 Advance podinfo.test canary weight 40
 Advance podinfo.test canary weight 45
 Advance podinfo.test canary weight 50
 Copying podinfo.test template spec to podinfo-primary.test
 Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
 Promotion completed! Scaling down podinfo.test
```

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

A canary deployment is triggered by changes in any of the following objects:

* Deployment PodSpec \(container image, command, ports, env, resources, etc\)
* ConfigMaps mounted as volumes or mapped to environment variables
* Secrets mounted as volumes or mapped to environment variables

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   15       2019-06-30T14:05:07Z
prod        frontend  Succeeded     0        2019-06-30T16:15:07Z
prod        backend   Failed        0        2019-06-30T17:05:07Z
```

## Automated rollback

During the canary analysis you can generate HTTP 500 errors and high latency to test if Flagger pauses and rolls back the faulted version.

Trigger another canary deployment:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.2
```

Exec into the load tester pod with:

```bash
kubectl -n test exec -it flagger-loadtester-xx-xx sh
```

Generate HTTP 500 errors:

```bash
watch -n 1 curl http://podinfo-canary.test:9898/status/500
```

Generate latency:

```bash
watch -n 1 curl http://podinfo-canary.test:9898/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary, the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:         0
  Failed Checks:         10
  Phase:                 Failed
Events:
 Starting canary analysis for podinfo.test
 Pre-rollout check acceptance-test passed
 Advance podinfo.test canary weight 5
 Advance podinfo.test canary weight 10
 Advance podinfo.test canary weight 15
 Halt podinfo.test advancement success rate 69.17% < 99%
 Halt podinfo.test advancement success rate 61.39% < 99%
 Halt podinfo.test advancement success rate 55.06% < 99%
 Halt podinfo.test advancement request duration 1.20s > 0.5s
 Halt podinfo.test advancement request duration 1.45s > 0.5s
 Rolling back podinfo.test failed checks threshold reached 5
 Canary failed! Scaling down podinfo.test
```

The above procedures can be extended with [custom metrics](../usage/metrics.md) checks, [webhooks](../usage/webhooks.md), [manual promotion](../usage/webhooks.md#manual-gating) approval and [Slack or MS Teams](../usage/alerting.md) notifications.
