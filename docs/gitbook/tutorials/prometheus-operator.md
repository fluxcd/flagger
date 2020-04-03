# Canary analysis with Prometheus Operator

This guide show you how to use Prometheus Operator for canary analysis.

## Prerequisites

Install Prometheus Operator with Helm v3:

```bash
helm repo add stable https://kubernetes-charts.storage.googleapis.com

kubectl create ns monitoring
helm upgrade -i prometheus stable/prometheus-operator \
--namespace monitoring \
--set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
--set fullnameOverride=prometheus
```

The `prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false`
option allows Prometheus operator to watch serviceMonitors outside of his namespace.

Install Flagger by setting the metrics server to Prometheus:

```bash
helm repo add flagger https://flagger.app

kubectl create ns flagger-system
helm upgrade -i flagger flagger/flagger \
--namespace flagger-system \
--set metricsServer=http://prometheus-prometheus.monitoring:9090 \
--set meshProvider=kubernetes
```

Install Flagger's tester:

```bash
helm upgrade -i loadtester flagger/loadtester \
--namespace flagger-system
```

Install podinfo demo app:

```bash
helm repo add podinfo https://stefanprodan.github.io/podinfo

kubectl create ns test
helm upgrade -i podinfo podinfo/podinfo \
--namespace test \
--set service.enabled=false
```

## Service monitors

The demo app is instrumented with Prometheus so you can create service monitors to scrape podinfo's metrics endpoint:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: podinfo-primary
  namespace: test
spec:
  endpoints:
  - path: /metrics
    port: http
    interval: 5s
  selector:
    matchLabels:
      app: podinfo
```

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: podinfo-canary
  namespace: test
spec:
  endpoints:
  - path: /metrics
    port: http
    interval: 5s
  selector:
    matchLabels:
      app: podinfo-canary
```

We are setting `interval: 5s` to have a more aggressive scraping.
If you do not define it, you must to use a longer interval in the Canary object.

## Metric templates

Create a metric template to measure the HTTP requests error rate:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: error-rate
  namespace: test
spec:
  provider:
    address: http://prometheus-prometheus.monitoring:9090
    type: prometheus
  query: |
    100 - rate(
      http_requests_total{
        namespace="{{ namespace }}",
        job="{{ target }}-canary",
        status!~"5.*"
      }[{{ interval }}]) 
    / 
    rate(
      http_requests_total{
        namespace="{{ namespace }}",
        job="{{ target }}-canary"
      }[{{ interval }}]
    ) * 100
```

Amd a metric template to measure the HTTP requests average duration:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: latency
  namespace: test
spec:
  provider:
    address: http://prometheus-prometheus.monitoring:9090
    type: prometheus
  query: |
    histogram_quantile(0.99,
      sum(
        rate(
          http_request_duration_seconds_bucket{
            namespace="{{ namespace }}",
            job="{{ target }}-canary"
          }[{{ interval }}]
        )
      ) by (le)
    )
```

## Canary analysis

Using the metrics template you can configure the canary analysis with HTTP error rate and latency checks:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: kubernetes
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: http
    name: podinfo
  analysis:
    interval: 30s
    iterations: 10
    threshold: 2
    metrics:
    - name: error-rate
      templateRef:
        name: error-rate
      thresholdRange:
        max: 1
      interval: 30s
    - name: latency
      templateRef:
        name: latency
      thresholdRange:
        max: 0.5
      interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: "http://loadtester.flagger-system/"
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test/"
```

Based on the above specification, Flagger creates the primary and canary Kubernetes ClusterIP service. 

During the canary analysis, Prometheus will scrape the canary service and Flagger will use the HTTP error rate and 
latency queries to determine if the release should be promoted or rolled back.
