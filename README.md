# flagger

[![build](https://img.shields.io/circleci/build/github/weaveworks/flagger/master.svg)](https://circleci.com/gh/weaveworks/flagger)
[![report](https://goreportcard.com/badge/github.com/weaveworks/flagger)](https://goreportcard.com/report/github.com/weaveworks/flagger)
[![codecov](https://codecov.io/gh/weaveworks/flagger/branch/master/graph/badge.svg)](https://codecov.io/gh/weaveworks/flagger)
[![license](https://img.shields.io/github/license/weaveworks/flagger.svg)](https://github.com/weaveworks/flagger/blob/master/LICENSE)
[![release](https://img.shields.io/github/release/weaveworks/flagger/all.svg)](https://github.com/weaveworks/flagger/releases)

Flagger is a progressive delivery tool that automates the release process for applications running on Kubernetes. 
It reduces the risk of introducing a new software version in production
by gradually shifting traffic to the new version while measuring metrics and running conformance tests.

![flagger-overview](https://raw.githubusercontent.com/weaveworks/flagger/master/docs/diagrams/flagger-canary-overview.png)

Flagger implements several deployment strategies (Canary releases, A/B testing, Blue/Green mirroring)
using a service mesh (App Mesh, Istio, Linkerd) or an ingress controller (Contour, Gloo, NGINX) for traffic routing.
For release analysis, Flagger can query Prometheus, Datadog or CloudWatch
and for alerting it uses Slack, MS Teams, Discord and Rocket.

### Documentation

Flagger documentation can be found at [docs.flagger.app](https://docs.flagger.app).

* Install
  * [Flagger install on Kubernetes](https://docs.flagger.app/install/flagger-install-on-kubernetes)
* Usage
  * [How it works](https://docs.flagger.app/usage/how-it-works)
  * [Deployment strategies](https://docs.flagger.app/usage/deployment-strategies)
  * [Metrics analysis](https://docs.flagger.app/usage/metrics)
  * [Webhooks](https://docs.flagger.app/usage/webhooks)
  * [Alerting](https://docs.flagger.app/usage/alerting)
  * [Monitoring](https://docs.flagger.app/usage/monitoring)
* Tutorials
  * [App Mesh](https://docs.flagger.app/tutorials/appmesh-progressive-delivery)
  * [Istio](https://docs.flagger.app/tutorials/istio-progressive-delivery)
  * [Linkerd](https://docs.flagger.app/tutorials/linkerd-progressive-delivery)
  * [Contour](https://docs.flagger.app/tutorials/contour-progressive-delivery)
  * [Gloo](https://docs.flagger.app/tutorials/gloo-progressive-delivery)
  * [NGINX Ingress](https://docs.flagger.app/tutorials/nginx-progressive-delivery)
  * [Kubernetes Blue/Green](https://docs.flagger.app/tutorials/kubernetes-blue-green)

### Who is using Flagger

List of organizations using Flagger:

* [Chick-fil-A](https://www.chick-fil-a.com)
* [Capra Consulting](https://www.capraconsulting.no)
* [DMM.com](https://dmm-corp.com)
* [MediaMarktSaturn](https://www.mediamarktsaturn.com)
* [Weaveworks](https://weave.works)

If you are using Flagger, please submit a PR to add your organization to the list!

### Canary CRD

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services, service mesh or ingress routes).
These objects expose the application on the mesh and drive the canary analysis and promotion.

Flagger keeps track of ConfigMaps and Secrets referenced by a Kubernetes Deployment and triggers a canary analysis if any of those objects change.
When promoting a workload in production, both code (container images) and configuration (config maps and secrets) are being synchronised.

For a deployment named _podinfo_, a canary promotion can be defined using Flagger's custom resource:

```yaml
apiVersion: flagger.app/v1beta1
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
  analysis:
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

For more details on how the canary analysis and promotion works please [read the docs](https://docs.flagger.app/usage/how-it-works).

### Features

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

### Roadmap

* Add support for Kubernetes [Ingress v2](https://github.com/kubernetes-sigs/service-apis)
* Integrate with other service mesh like Consul Connect and ingress controllers like HAProxy, ALB
* Integrate with other metrics providers like InfluxDB, Stackdriver, SignalFX
* Add support for comparing the canary metrics to the primary ones and do the validation based on the derivation between the two

### Contributing

Flagger is Apache 2.0 licensed and accepts contributions via GitHub pull requests.
To start contributing please read the [development guide](https://docs.flagger.app/dev/dev-guide).

When submitting bug reports please include as much details as possible:

* which Flagger version
* which Flagger CRD version
* which Kubernetes version
* what configuration (canary, ingress and workloads definitions)
* what happened (Flagger and Proxy logs)

### Getting Help

If you have any questions about Flagger and progressive delivery:

* Read the Flagger [docs](https://docs.flagger.app).
* Invite yourself to the [Weave community slack](https://slack.weave.works/)
  and join the [#flagger](https://weave-community.slack.com/messages/flagger/) channel.
* Join the [Weave User Group](https://www.meetup.com/pro/Weave/) and get invited to online talks,
  hands-on training and meetups in your area.
* File an [issue](https://github.com/weaveworks/flagger/issues/new).

Your feedback is always welcome!
