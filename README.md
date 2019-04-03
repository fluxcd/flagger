# flagger

[![build](https://travis-ci.org/weaveworks/flagger.svg?branch=master)](https://travis-ci.org/weaveworks/flagger)
[![report](https://goreportcard.com/badge/github.com/weaveworks/flagger)](https://goreportcard.com/report/github.com/weaveworks/flagger)
[![codecov](https://codecov.io/gh/weaveworks/flagger/branch/master/graph/badge.svg)](https://codecov.io/gh/weaveworks/flagger)
[![license](https://img.shields.io/github/license/weaveworks/flagger.svg)](https://github.com/weaveworks/flagger/blob/master/LICENSE)
[![release](https://img.shields.io/github/release/weaveworks/flagger/all.svg)](https://github.com/weaveworks/flagger/releases)

Flagger is a Kubernetes operator that automates the promotion of canary deployments
using Istio or App Mesh routing for traffic shifting and Prometheus metrics for canary analysis.
The canary analysis can be extended with webhooks for running acceptance tests,
load tests or any other custom validation.

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance
indicators like HTTP requests success rate, requests average duration and pods health.
Based on analysis of the KPIs a canary is promoted or aborted, and the analysis result is published to Slack.

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
* Usage
  * [Istio canary deployments](https://docs.flagger.app/usage/progressive-delivery)
  * [Istio A/B testing](https://docs.flagger.app/usage/ab-testing)
  * [App Mesh canary deployments](https://docs.flagger.app/usage/appmesh-progressive-delivery)
  * [Monitoring](https://docs.flagger.app/usage/monitoring)
  * [Alerting](https://docs.flagger.app/usage/alerting)
* Tutorials
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
    # HTTP match conditions (optional)
    match:
      - uri:
          prefix: /
    # HTTP rewrite (optional)
    rewrite:
      uri: /
    # Envoy timeout and retry policy (optional)
    headers:
      request:
        add:
          x-envoy-upstream-rq-timeout-ms: "15000"
          x-envoy-max-retries: "10"
          x-envoy-retry-on: "gateway-error,connect-failure,refused-stream"
    # cross-origin resource sharing policy (optional)
    corsPolicy:
      allowOrigin:
        - example.com
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
    # Istio Prometheus checks
    metrics:
    # builtin Istio checks
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
    # custom check
    - name: "kafka lag"
      threshold: 100
      query: |
        avg_over_time(
          kafka_consumergroup_lag{
            consumergroup=~"podinfo-consumer-.*",
            topic="podinfo"
          }[1m]
        )
    # external checks (optional)
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo.test:9898/"
```

For more details on how the canary analysis and promotion works please [read the docs](https://docs.flagger.app/how-it-works).

## Features

| Feature                                      | Istio              | App Mesh           |
| -------------------------------------------- | ------------------ | ------------------ |
| Canary deployments (weighted traffic)        | :heavy_check_mark: | :heavy_check_mark: |
| A/B testing (headers and cookies filters)    | :heavy_check_mark: | :heavy_minus_sign: |
| Load testing                                 | :heavy_check_mark: | :heavy_check_mark: |
| Webhooks (custom acceptance tests)           | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (Envoy metric)    | :heavy_check_mark: | :heavy_check_mark: |
| Request duration check (Envoy metric)        | :heavy_check_mark: | :heavy_minus_sign: |
| Custom promql checks                         | :heavy_check_mark: | :heavy_check_mark: |
| Ingress gateway (CORS, retries and timeouts) | :heavy_check_mark: | :heavy_minus_sign: |

## Frequently asked questions

**Can Flagger be part of my integration tests?**
> Yes, Flagger supports webhooks to do integration testing.

**What if I only want to target beta testers?**
> That's a feature in Flagger, not in App Mesh. It's on the App Mesh roadmap.

**When do I use A/B testing when Canary?**
> One advantage of using A/B testing is that each version remains separated and routes aren't mixed.
>
> Using a Canary deployment can lead to behaviour like this one observed by a
> user:
>
> [..] during a canary deployment of our nodejs app, the version that is being served <50% traffic reports mime type mismatch errors in the browser (js as "text/html")
> When the deployment Passes/ Fails (doesn't really matter) the version that stays alive works as expected. If anyone has any tips or direction I would greatly appreciate it. Even if its as simple as I'm looking in the wrong place. Thanks in advance!
>
> The issue was that we were not maintaining session affinity while serving files for our frontend. Which resulted in any redirects or refreshes occasionally returning a mismatched app.*.js file (generated from vue)
>
> Read up on [A/B testing](https://docs.flagger.app/usage/ab-testing).

## Roadmap

* Integrate with other service mesh technologies like Linkerd v2, Super Gloo or Consul Mesh
* Add support for comparing the canary metrics to the primary ones and do the validation based on the derivation between the two

## Contributing

Flagger is Apache 2.0 licensed and accepts contributions via GitHub pull requests.

When submitting bug reports please include as much details as possible:

* which Flagger version
* which Flagger CRD version
* which Kubernetes/Istio version
* what configuration (canary, virtual service and workloads definitions)
* what happened (Flagger, Istio Pilot and Proxy logs)

## Getting Help

If you have any questions about Flagger and progressive delivery:

* Read the Flagger [docs](https://docs.flagger.app).
* Invite yourself to the [Weave community slack](https://slack.weave.works/)
  and join the [#flagger](https://weave-community.slack.com/messages/flagger/) channel.
* Join the [Weave User Group](https://www.meetup.com/pro/Weave/) and get invited to online talks,
  hands-on training and meetups in your area.
* File an [issue](https://github.com/weaveworks/flagger/issues/new).

Your feedback is always welcome!
