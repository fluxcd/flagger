# flagger

[![build](https://img.shields.io/circleci/build/github/weaveworks/flagger/master.svg)](https://circleci.com/gh/weaveworks/flagger)
[![report](https://goreportcard.com/badge/github.com/weaveworks/flagger)](https://goreportcard.com/report/github.com/weaveworks/flagger)
[![codecov](https://codecov.io/gh/weaveworks/flagger/branch/master/graph/badge.svg)](https://codecov.io/gh/weaveworks/flagger)
[![license](https://img.shields.io/github/license/weaveworks/flagger.svg)](https://github.com/weaveworks/flagger/blob/master/LICENSE)
[![release](https://img.shields.io/github/release/weaveworks/flagger/all.svg)](https://github.com/weaveworks/flagger/releases)

Flagger is a Kubernetes operator that automates the promotion of canary deployments
using Istio, Linkerd, App Mesh, NGINX, Contour or Gloo routing for traffic shifting and Prometheus metrics for canary analysis.
The canary analysis can be extended with webhooks for running acceptance tests,
load tests or any other custom validation.

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance
indicators like HTTP requests success rate, requests average duration and pods health.
Based on analysis of the KPIs a canary is promoted or aborted, and the analysis result is published to Slack or MS Teams.

![flagger-overview](https://raw.githubusercontent.com/weaveworks/flagger/master/docs/diagrams/flagger-canary-overview.png)

## Documentation

Flagger documentation can be found at [docs.flagger.app](https://docs.flagger.app)

* Install
  * [Flagger install on Kubernetes](https://docs.flagger.app/install/flagger-install-on-kubernetes)
  * [Flagger install on GKE Istio](https://docs.flagger.app/install/flagger-install-on-google-cloud)
  * [Flagger install on EKS App Mesh](https://docs.flagger.app/install/flagger-install-on-eks-appmesh)
* How it works
  * [Canary custom resource](https://docs.flagger.app/how-it-works#canary-custom-resource)
  * [Routing](https://docs.flagger.app/how-it-works#istio-routing)
  * [Canary deployment stages](https://docs.flagger.app/how-it-works#canary-deployment)
  * [Canary analysis](https://docs.flagger.app/how-it-works#canary-analysis)
  * [HTTP metrics](https://docs.flagger.app/how-it-works#http-metrics)
  * [Custom metrics](https://docs.flagger.app/how-it-works#custom-metrics)
  * [Webhooks](https://docs.flagger.app/how-it-works#webhooks)
  * [Load testing](https://docs.flagger.app/how-it-works#load-testing)
  * [Manual gating](https://docs.flagger.app/how-it-works#manual-gating)
  * [FAQ](https://docs.flagger.app/faq)
  * [Development guide](https://docs.flagger.app/dev-guide)
* Usage
  * [Deployment Strategies](https://docs.flagger.app/usage/deployment-strategies)
  * [Monitoring](https://docs.flagger.app/usage/monitoring)
  * [Alerting](https://docs.flagger.app/usage/alerting)
* Tutorials
  * [Istio Canary Deployments](https://docs.flagger.app/tutorials/istio-progressive-delivery)
  * [Istio A/B Testing](https://docs.flagger.app/tutorials/istio-ab-testing)
  * [Linkerd Canary Deployments](https://docs.flagger.app/tutorials/linkerd-progressive-delivery)
  * [App Mesh Canary Deployments](https://docs.flagger.app/tutorials/appmesh-progressive-delivery)
  * [NGINX Canary Deployments](https://docs.flagger.app/tutorials/nginx-progressive-delivery)
  * [Gloo Canary Deployments](https://docs.flagger.app/tutorials/gloo-progressive-delivery)
  * [Contour Canary Deployments](https://docs.flagger.app/tutorials/contour-progressive-delivery)
  * [Kubernetes Blue/Green Deployments](https://docs.flagger.app/tutorials/kubernetes-blue-green)
  * [Canary deployments with Helm charts and Weave Flux](https://docs.flagger.app/tutorials/canary-helm-gitops)

## Canary CRD

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services and Istio or App Mesh virtual services).
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
  # service mesh provider (optional)
  # can be: kubernetes, istio, linkerd, appmesh, nginx, contour, gloo, supergloo
  provider: istio
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
    # service name (defaults to targetRef.name)
    name: podinfo
    # ClusterIP port number
    port: 9898
    # container port name or number (optional)
    targetPort: 9898
    # port name can be http or grpc (default http)
    portName: http
    # add all the other container ports
    # to the ClusterIP services (default false)
    portDiscovery: true
    # HTTP match conditions (optional)
    match:
      - uri:
          prefix: /
    # HTTP rewrite (optional)
    rewrite:
      uri: /
    # request timeout (optional)
    timeout: 5s
  # promote the canary without analysing it (default false)
  skipAnalysis: false
  # define the canary analysis timing and KPIs
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
    # validation (optional)
    metrics:
    - name: request-success-rate
      # builtin Prometheus check
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      threshold: 99
      interval: 1m
    - name: request-duration
      # builtin Prometheus check
      # maximum req duration P99
      # milliseconds
      threshold: 500
      interval: 30s
    - name: "database connections"
      # custom Prometheus check
      templateRef:
        name: db-connections
      thresholdRange:
        min: 2
        max: 100
      interval: 1m
    # testing (optional)
    webhooks:
      - name: "conformance test"
        type: pre-rollout
        url: http://flagger-helmtester.test/
        timeout: 5m
        metadata:
          type: "helmv3"
          cmd: "test run podinfo -n test"
      - name: "load test"
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo.test:9898/"
    # alerting (optional)
    alerts:
      - name: "dev team Slack"
        severity: error
        providerRef:
          name: dev-slack
          namespace: flagger
      - name: "qa team Discord"
        severity: warn
        providerRef:
          name: qa-discord
      - name: "on-call MS Teams"
        severity: info
        providerRef:
          name: on-call-msteams
```

For more details on how the canary analysis and promotion works please [read the docs](https://docs.flagger.app/how-it-works).

## Features

| Feature                                      | Istio              | Linkerd            | App Mesh           | NGINX              | Gloo               | Contour            | CNI                |
| -------------------------------------------- | ------------------ | ------------------ |------------------  |------------------  |------------------  |------------------  |------------------  |
| Canary deployments (weighted traffic)        | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| A/B testing (headers and cookies routing)    | :heavy_check_mark: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_minus_sign: |
| Blue/Green deployments (traffic switch)      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Webhooks (acceptance/load testing)           | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Manual gating (approve/pause/resume)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (L7 metric)       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| Request duration check (L7 metric)           | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| Custom promql checks                         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Traffic policy, CORS, retries and timeouts   | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_minus_sign: |

## Roadmap

* Integrate with other service mesh like Consul Connect and ingress controllers like HAProxy, ALB
* Add support for comparing the canary metrics to the primary ones and do the validation based on the derivation between the two

## Contributing

Flagger is Apache 2.0 licensed and accepts contributions via GitHub pull requests.

When submitting bug reports please include as much details as possible:

* which Flagger version
* which Flagger CRD version
* which Kubernetes version
* what configuration (canary, ingress and workloads definitions)
* what happened (Flagger and Proxy logs)

## Getting Help

If you have any questions about Flagger and progressive delivery:

* Read the Flagger [docs](https://docs.flagger.app).
* Invite yourself to the [Weave community slack](https://slack.weave.works/)
  and join the [#flagger](https://weave-community.slack.com/messages/flagger/) channel.
* Join the [Weave User Group](https://www.meetup.com/pro/Weave/) and get invited to online talks,
  hands-on training and meetups in your area.
* File an [issue](https://github.com/weaveworks/flagger/issues/new).

Your feedback is always welcome!