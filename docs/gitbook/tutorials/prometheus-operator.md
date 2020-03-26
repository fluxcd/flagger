# Flagger with Prometheus Operator

This guide will show you how to use Flagger and Prometheus Operator.  
This guide will handle only Blue/Green Deployment with podinfo application

## Prerequisites

Flagger and Prometheus Operator requires a Kubernetes cluster **v1.11** or newer

Install Prometheus-Operator with Helm v3:

```bash
helm repo add stable https://kubernetes-charts.storage.googleapis.com
helm repo update
kubectl create ns prometheus
helm upgrade -i prometheus stable/prometheus-operator \
--namespace prometheus \
--set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
```

The `prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false` option allows Prometheus-Operator to watch serviceMonitor outside of his namespace.

You can also set `prometheus.service.type=nodePort` if you want to have

Install Flagger with Helm v3:

```bash
helm repo add flagger https://flagger.app
helm repo update
kubectl create ns flagger
helm upgrade -i flagger flagger/flagger \
--namespace flagger \
--set metricsServer=http://prometheus-prometheus-oper-prometheus.prometheus:9090 \
--set meshProvider=kubernetes
```

The `meshProvider` option can be changed to your value, if you want to do something else than Blue/Green Deployment

Install Flagger Loadtester with Helm v3:

```bash
helm repo add flagger https://flagger.app
helm repo update
kubectl create ns flagger
helm upgrade -i loadtester flagger/loadtester \
--namespace flagger
```

Install podinfo with Helm v3:

```bash
helm repo add sp https://stefanprodan.github.io/podinfo
helm repo update
kubectl create ns test
helm upgrade -i podinfo sp/podinfo \
--namespace test
```

## Setting ServiceMonitor

Prometheus Operator is using mostly serviceMonitor instead of annotations.  
In order to catch metrics for primary and canary service, you will need to create 2 serviceMonitors :

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: podinfo
  namespace: test
spec:
  endpoints:
  - path: /metrics
    port: http
    interval: 15s
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
    interval: 15s
  selector:
    matchLabels:
      app: podinfo-canary
```

We are setting `interval: 15s` to have a more aggressive scraping
If you do not define it, you must to use a longer interval in the Canary object

## Setting Custom metrics

Prometheus Operator is relabeling for every serviceMonitor, you can create custom metrics to you own filter.

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: request-success-rate
  namespace: test
spec:
  provider:
    address: http://prometheus-prometheus-oper-prometheus.prometheus:9090
    type: prometheus
  query: rate(http_requests_total{namespace="{{ namespace }}",job="{{ target }}-canary",status!~"5.*"}[{{ interval }}]) / rate(http_requests_total{namespace="{{ namespace }}",job="{{ target }}-canary"}[{{ interval }}]) * 100
```

You can also use `pod="{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"` instead of `job={{ target }}-canary`, if you want.

## Creating Canary

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
    port: 9898
    portDiscovery: true
  analysis:
    # We set a longer interval to let Prometheus fetch metrics
    interval: 30s
    iterations: 10
    threshold: 2
    metrics:
    # For some reason, you need to not use a standard name
    - name: my-custom-metrics
      templateRef:
        name: request-success-rate
        namespace: test
      thresholdRange:
        min: 99
      interval: 1m
    webhooks:
      - name: smoke-test
        type: pre-rollout
        url: "http://loadtester.flagger/"
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'anon' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: "http://loadtester.flagger/"
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z {{ interval }} -q 10 -c 2 http://podinfo-canary.test:9898"
```

## Test the canary

Execute `kubectl -n test set image deployment/podinfo podinfo=stefanprodan/podinfo:3.1.0` to see if everythinf works



