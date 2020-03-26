# Flagger with Prometheus Operator

This guide will show you how to use Flagger and Prometheus Operator
This guide will handle only Blue/Green Deployment

## Prerequisites

Flagger requires a Kubernetes cluster **v1.11** or newer and NGINX ingress **0.24** or newer.

Install Prometheus-Operator with Helm v3:

```bash
helm repo add stable https://kubernetes-charts.storage.googleapis.com
helm repo update
kubectl create ns prometheus
helm upgrade -i prometheus stable/prometheus-operator \
--namespace prometheus \
--set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValue=false
```
The `prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValue` allows Prometheus-Operator to watch serviceMonitor outside of his namespace.

Install Flagger with Helm v3:

```bash
helm repo add flagger https://flagger.app
helm repo update
kubectl create ns flagger
helm upgrade -i flagger flagger/flagger \
--namespace flagger
--set prometheus-prometheus-oper-prometheus.prometheus:9090 \
--set meshProvider=kubernetes
```

The `meshProvider` option can be changed to your value

## Setting ServiceMonitor

Prometheus Operator is using mostly serviceMonitor instead of annotations.
In order to catch metrics for primary and canary service, you will need to create 2 serviceMonitors :

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: service-monitor-primary
  namespace: application
spec:
  endpoints:
  - path: /metrics
    port: http
  selector:
    matchLabels:
      app: application
```

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: service-monitor-canary
  namespace: application
spec:
  endpoints:
  - path: /metrics
    port: http
  selector:
    matchLabels:
      app: application-canary
```

## Setting Custom metrics

Prometheus Operator is relabeling for every serviceMonitor, you can create custom metrics

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: your-custom-metrics
  namespace: application
spec:
  provider:
    address: http://prometheus-prometheus-oper-prometheus.prometheus:9090
    type: prometheus
  query: |
    100 - sum(
          rate(
            http_server_requests_seconds_max{
              namespace="{{namespace }}",
            job="{{ target }}-canary",
            status!~\"5.*\"
        }[{{ interval }}]
        )
      ) 
      / 
      sum(
        rate(
            http_server_requests_seconds_max{
            namespace="{{ namespace }}",
            job="{{ target }}-canary"
        }[{{ interval }}]
        )
      ) 
      * 100
```

You can use the job to filter instead of pod. You can tune it to your installation.

## Creating Canary

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: application
  namespace: security-manager
spec:
  analysis:
    interval: 30s
    iterations: 10
    threshold: 5
    metrics:
    - interval: 1m
      name: http-requests-seconds
      templateRef:
        name: your-custom-metrics
        namespace: application
      thresholdRange:
        max: 1
    webhooks: []
  progressDeadlineSeconds: 600
  provider: kubernetes
  service:
    port: 8080
    portDiscovery: true
  targetRef:
    apiVersion: v1
    kind: Deployment
    name: application
```

You can define the webhooks of your liking also


