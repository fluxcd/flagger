# Apache APISIX Canary Deployments

This guide shows you how to use the [Apache APISIX](https://apisix.apache.org/) and Flagger to automate canary deployments.

![Flagger Apache APISIX Overview](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-apisix-overview.png)

## Prerequisites

Flagger requires a Kubernetes cluster **v1.19** or newer and Apache APISIX **v2.15** or newer and Apache APISIX Ingress Controller **v1.5.0** or newer.

Install Apache APISIX and Apache APISIX Ingress Controller with Helm v3:

```bash
helm repo add apisix https://charts.apiseven.com
kubectl create ns apisix

helm upgrade -i apisix apisix/apisix --version=0.11.3 \
--namespace apisix \
--set apisix.podAnnotations."prometheus\.io/scrape"=true \
--set apisix.podAnnotations."prometheus\.io/port"=9091 \
--set apisix.podAnnotations."prometheus\.io/path"=/apisix/prometheus/metrics \
--set pluginAttrs.prometheus.export_addr.ip=0.0.0.0 \
--set pluginAttrs.prometheus.export_addr.port=9091 \
--set pluginAttrs.prometheus.export_uri=/apisix/prometheus/metrics \
--set pluginAttrs.prometheus.metric_prefix=apisix_ \
--set ingress-controller.enabled=true \
--set ingress-controller.config.apisix.serviceNamespace=apisix
```

Install Flagger and the Prometheus add-on in the same namespace as Apache APISIX:

```bash
helm repo add flagger https://flagger.app

helm upgrade -i flagger flagger/flagger \
--namespace apisix \
--set prometheus.install=true \
--set meshProvider=apisix
```

## Bootstrap

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler \(HPA\), then creates a series of objects \(Kubernetes deployments, ClusterIP services and an ApisixRoute\). These objects expose the application outside the cluster and drive the canary analysis and promotion.

Create a test namespace:

```bash
kubectl create ns test
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/podinfo?ref=main
```

Deploy the load testing service to generate traffic during the canary analysis:

```bash
helm upgrade -i flagger-loadtester flagger/loadtester \
--namespace=test
```

Create an Apache APISIX `ApisixRoute`, Flagger will reference and generate the canary Apache APISIX `ApisixRoute` \(replace `app.example.com` with your own domain\):

```yaml
apiVersion: apisix.apache.org/v2
kind: ApisixRoute
metadata:
  name: podinfo
  namespace: test
spec:
  http:
    - backends:
        - serviceName: podinfo
          servicePort: 80
      match:
        hosts:
          - app.example.com
        methods:
          - GET
        paths:
          - /*
      name: method
      plugins:
        - name: prometheus
          enable: true
          config:
            disable: false
            prefer_name: true
```

Save the above resource as podinfo-apisixroute.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-apisixroute.yaml
```

Create a canary custom resource \(replace `app.example.com` with your own domain\):

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: apisix
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  # apisix route reference
  routeRef:
    apiVersion: apisix.apache.org/v2
    kind: ApisixRoute
    name: podinfo
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  service:
    # ClusterIP port number
    port: 80
    # container port number or name
    targetPort: 9898
  analysis:
    # schedule interval (default 60s)
    interval: 10s
    # max number of failed metric checks before rollback
    threshold: 10
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 10
    # APISIX Prometheus checks
    metrics:
      - name: request-success-rate
        # minimum req success rate (non 5xx responses)
        # percentage (0-100)
        thresholdRange:
          min: 99
        interval: 1m
      - name: request-duration
        # builtin Prometheus check
        # maximum req duration P99
        # milliseconds
        thresholdRange:
          max: 500
        interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        type: rollout
        metadata:
          cmd: |-
            hey -z 1m -q 10 -c 2 -h2 -host app.example.com http://apisix-gateway.apisix/api/info
```

Save the above resource as podinfo-canary.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

After a couple of seconds Flagger will create the canary objects:

```bash
# applied 
deployment.apps/podinfo
horizontalpodautoscaler.autoscaling/podinfo
apisixroute/podinfo
canary.flagger.app/podinfo

# generated 
deployment.apps/podinfo-primary
horizontalpodautoscaler.autoscaling/podinfo-primary
service/podinfo
service/podinfo-canary
service/podinfo-primary
apisixroute/podinfo-podinfo-canary
```

## Automated canary promotion

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance indicators like HTTP requests success rate, requests average duration and pod health. Based on analysis of the KPIs a canary is promoted or aborted, and the analysis result is published to Slack or MS Teams.

![Flagger Canary Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-canary-steps.png)

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=stefanprodan/podinfo:6.0.1
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:  0
  Conditions:
    Message:               Canary analysis completed successfully, promotion finished.
    Reason:                Succeeded
    Status:                True
    Type:                  Promoted
  Failed Checks:           1
  Iterations:              0
  Phase:                   Succeeded

Events:
  Type     Reason  Age                    From     Message
  ----     ------  ----                   ----     -------
  Warning  Synced  2m59s                  flagger  podinfo-primary.test not ready: waiting for rollout to finish: observed deployment generation less than desired generation
  Warning  Synced  2m50s                  flagger  podinfo-primary.test not ready: waiting for rollout to finish: 0 of 1 (readyThreshold 100%) updated replicas are available
  Normal   Synced  2m40s (x3 over 2m59s)  flagger  all the metrics providers are available!
  Normal   Synced  2m39s                  flagger  Initialization done! podinfo.test
  Normal   Synced  2m20s                  flagger  New revision detected! Scaling up podinfo.test
  Warning  Synced  2m (x2 over 2m10s)     flagger  canary deployment podinfo.test not ready: waiting for rollout to finish: 0 of 1 (readyThreshold 100%) updated replicas are available
  Normal   Synced  110s                   flagger  Starting canary analysis for podinfo.test
  Normal   Synced  109s                   flagger  Advance podinfo.test canary weight 10
  Warning  Synced  100s                   flagger  Halt advancement no values found for apisix metric request-success-rate probably podinfo.test is not receiving traffic: running query failed: no values found
  Normal   Synced  90s                    flagger  Advance podinfo.test canary weight 20
  Normal   Synced  80s                    flagger  Advance podinfo.test canary weight 30
  Normal   Synced  69s                    flagger  Advance podinfo.test canary weight 40
  Normal   Synced  59s                    flagger  Advance podinfo.test canary weight 50
  Warning  Synced  30s (x2 over 40s)      flagger  podinfo-primary.test not ready: waiting for rollout to finish: 1 old replicas are pending termination
  Normal   Synced  9s (x3 over 50s)       flagger  (combined from similar events): Promotion completed! Scaling down podinfo.test
```

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS      WEIGHT   LASTTRANSITIONTIME
test        podinfo-2   Progressing   10       2022-11-23T05:00:54Z
test        podinfo     Succeeded     0        2022-11-23T06:00:54Z
```

## Automated rollback

During the canary analysis you can generate HTTP 500 errors to test if Flagger pauses and rolls back the faulted version.

Trigger another canary deployment:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=stefanprodan/podinfo:6.0.2
```

Exec into the load tester pod with:

```bash
kubectl -n test exec -it deploy/flagger-loadtester bash
```

Generate HTTP 500 errors:

```bash
hey -z 1m -c 5 -q 5 -host app.example.com http://apisix-gateway.apisix/status/500
```

Generate latency:

```bash
watch -n 1 curl -H \"host: app.example.com\" http://apisix-gateway.apisix/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary, the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n apisix logs deploy/flagger -f | jq .msg

"New revision detected! Scaling up podinfo.test"
"canary deployment podinfo.test not ready: waiting for rollout to finish: 0 of 1 (readyThreshold 100%) updated replicas are available"
"Starting canary analysis for podinfo.test"
"Advance podinfo.test canary weight 10"
"Halt podinfo.test advancement success rate 0.00% < 99%"
"Halt podinfo.test advancement success rate 26.76% < 99%"
"Halt podinfo.test advancement success rate 34.19% < 99%"
"Halt podinfo.test advancement success rate 37.32% < 99%"
"Halt podinfo.test advancement success rate 39.04% < 99%"
"Halt podinfo.test advancement success rate 40.13% < 99%"
"Halt podinfo.test advancement success rate 48.28% < 99%"
"Halt podinfo.test advancement success rate 50.35% < 99%"
"Halt podinfo.test advancement success rate 56.92% < 99%"
"Halt podinfo.test advancement success rate 67.70% < 99%"
"Rolling back podinfo.test failed checks threshold reached 10"
"Canary failed! Scaling down podinfo.test"
```

## Custom metrics

The canary analysis can be extended with Prometheus queries.

Create a metric template and apply it on the cluster:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: not-found-percentage
  namespace: test
spec:
  provider:
    type: prometheus
    address: http://flagger-prometheus.apisix:9090
  query: |
    sum(
      rate(
        apisix_http_status{
          route=~"{{ namespace }}_{{ route }}-{{ target }}-canary_.+",
          code!~"4.."
        }[{{ interval }}]
      )
    )
    /
    sum(
      rate(
        apisix_http_status{
          route=~"{{ namespace }}_{{ route }}-{{ target }}-canary_.+"
        }[{{ interval }}]
      )
    ) * 100
```

Edit the canary analysis and add the not found error rate check:

```yaml
  analysis:
    metrics:
      - name: "404s percentage"
        templateRef:
          name: not-found-percentage
        thresholdRange:
          max: 5
        interval: 1m
```

The above configuration validates the canary by checking if the HTTP 404 req/sec percentage is below 5 percent of the total traffic. If the 404s rate reaches the 5% threshold, then the canary fails.


The above procedures can be extended with more [custom metrics](../usage/metrics.md) checks, [webhooks](../usage/webhooks.md), [manual promotion](../usage/webhooks.md#manual-gating) approval and [Slack or MS Teams](../usage/alerting.md) notifications.

