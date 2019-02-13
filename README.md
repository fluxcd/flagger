# flagger

[![build](https://travis-ci.org/stefanprodan/flagger.svg?branch=master)](https://travis-ci.org/stefanprodan/flagger)
[![report](https://goreportcard.com/badge/github.com/stefanprodan/flagger)](https://goreportcard.com/report/github.com/stefanprodan/flagger)
[![codecov](https://codecov.io/gh/stefanprodan/flagger/branch/master/graph/badge.svg)](https://codecov.io/gh/stefanprodan/flagger)
[![license](https://img.shields.io/github/license/stefanprodan/flagger.svg)](https://github.com/stefanprodan/flagger/blob/master/LICENSE)
[![release](https://img.shields.io/github/release/stefanprodan/flagger/all.svg)](https://github.com/stefanprodan/flagger/releases)

Flagger is a Kubernetes operator that automates the promotion of canary deployments
using Istio routing for traffic shifting and Prometheus metrics for canary analysis. 
The canary analysis can be extended with webhooks for running acceptance tests, 
load tests or any other custom validation.

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance 
indicators like HTTP requests success rate, requests average duration and pods health. 
Based on analysis of the KPIs a canary is promoted or aborted, and the analysis result is published to Slack.

![flagger-overview](https://raw.githubusercontent.com/stefanprodan/flagger/master/docs/diagrams/flagger-canary-overview.png)

### Documentation

Flagger documentation can be found at [docs.flagger.app](https://docs.flagger.app)

* Install
    * [Flagger install on Kubernetes](https://docs.flagger.app/install/flagger-install-on-kubernetes)
    * [Flagger install on GKE](https://docs.flagger.app/install/flagger-install-on-google-cloud)
* How it works
    * [Canary custom resource](https://docs.flagger.app/how-it-works#canary-custom-resource)
    * [Canary deployment stages](https://docs.flagger.app/how-it-works#canary-deployment)
    * [Canary analysis](https://docs.flagger.app/how-it-works#canary-analysis)
    * [HTTP metrics](https://docs.flagger.app/how-it-works#http-metrics)
    * [Webhooks](https://docs.flagger.app/how-it-works#webhooks)
    * [Load testing](https://docs.flagger.app/how-it-works#load-testing)
* Usage
    * [Canary promotions and rollbacks](https://docs.flagger.app/usage/progressive-delivery)
    * [Monitoring](https://docs.flagger.app/usage/monitoring)
    * [Alerting](https://docs.flagger.app/usage/alerting)
* Tutorials
    * [Canary deployments with Helm charts and Weave Flux](https://docs.flagger.app/tutorials/canary-helm-gitops)

### Install 

Before installing Flagger make sure you have Istio setup up with Prometheus enabled. 
If you are new to Istio you can follow my [Istio service mesh walk-through](https://github.com/stefanprodan/istio-gke).

Deploy Flagger in the `istio-system` namespace using Helm:

```bash
# add the Helm repository
helm repo add flagger https://flagger.app

# install or upgrade
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 
```

Flagger is compatible with Kubernetes >1.11.0 and Istio >1.0.0.

### Canary CRD

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services and Istio virtual services).
These objects expose the application on the mesh and drive the canary analysis and promotion.

Flagger keeps track of ConfigMaps and Secrets referenced by a Kubernetes Deployment and triggers a canary analysis if any of those objects change. 
When promoting a workload in production, both code (container images) and configuration (config maps and secrets) are being synchronised.

For a deployment named _podinfo_, a canary promotion can be defined using Flagger's custom resource:

```yaml
apiVersion: flagger.app/v1alpha3
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  # deployment reference
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  # HPA reference (optional)
  autoscalerRef:
    apiVersion: autoscaling/v2beta1
    kind: HorizontalPodAutoscaler
    name: podinfo
  service:
    # container port
    port: 9898
    # Istio gateways (optional)
    gateways:
    - public-gateway.istio-system.svc.cluster.local
    # Istio virtual service host names (optional)
    hosts:
    - podinfo.example.com
  # for emergency cases when you want to ship changes
  # in production without analysing the canary
  skipAnalysis: false
  canaryAnalysis:
    # schedule interval (default 60s)
    interval: 1m
    # max number of failed metric checks before rollback
    threshold: 10
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 5
    # Istio Prometheus checks
    metrics:
    - name: istio_requests_total
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      threshold: 99
      interval: 1m
    - name: istio_request_duration_seconds_bucket
      # maximum req duration P99
      # milliseconds
      threshold: 500
      interval: 30s
    # external checks (optional)
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo.test:9898/"
```

For more details on how the canary analysis and promotion works please [read the docs](https://docs.flagger.app/how-it-works).

### Roadmap

* Extend the validation mechanism to support other metrics than HTTP success rate and latency
* Add support for comparing the canary metrics to the primary ones and do the validation based on the derivation between the two
* Extend the canary analysis and promotion to other types than Kubernetes deployments such as Flux Helm releases or OpenFaaS functions

### Contributing

Flagger is Apache 2.0 licensed and accepts contributions via GitHub pull requests.

When submitting bug reports please include as much details as possible: 
* which Flagger version
* which Flagger CRD version
* which Kubernetes/Istio version
* what configuration (canary, virtual service and workloads definitions)
* what happened (Flagger, Istio Pilot and Proxy logs)
