  # flagger

[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4783/badge)](https://bestpractices.coreinfrastructure.org/projects/4783)
[![build](https://github.com/fluxcd/flagger/workflows/build/badge.svg)](https://github.com/fluxcd/flagger/actions)
[![report](https://goreportcard.com/badge/github.com/fluxcd/flagger)](https://goreportcard.com/report/github.com/fluxcd/flagger)
[![license](https://img.shields.io/github/license/fluxcd/flagger.svg)](https://github.com/fluxcd/flagger/blob/main/LICENSE)
[![release](https://img.shields.io/github/release/fluxcd/flagger/all.svg)](https://github.com/fluxcd/flagger/releases)

Flagger is a progressive delivery tool that automates the release process for applications running on Kubernetes. 
It reduces the risk of introducing a new software version in production
by gradually shifting traffic to the new version while measuring metrics and running conformance tests.

![flagger-overview](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-overview.png)

Flagger implements several deployment strategies (Canary releases, A/B testing, Blue/Green mirroring)
using a service mesh (App Mesh, Istio, Linkerd, Open Service Mesh)
or an ingress controller (Contour, Gloo, NGINX, Skipper, Traefik) for traffic routing.
For release analysis, Flagger can query Prometheus, Datadog, New Relic, CloudWatch, Dynatrace,
InfluxDB and Stackdriver and for alerting it uses Slack, MS Teams, Discord, Rocket and Google Chat.

Flagger is a [Cloud Native Computing Foundation](https://cncf.io/) project
and part of [Flux](https://fluxcd.io) family of GitOps tools.

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
  * [Skipper](https://docs.flagger.app/tutorials/skipper-progressive-delivery)
  * [Traefik](https://docs.flagger.app/tutorials/traefik-progressive-delivery)
  * [Open Service Mesh (OSM)](https://docs.flagger.app/tutorials/osm-progressive-delivery)
  * [Kubernetes Blue/Green](https://docs.flagger.app/tutorials/kubernetes-blue-green)

### Who is using Flagger

**Our list of production users has moved to <https://fluxcd.io/adopters/#flagger>**.

If you are using Flagger, please [submit a PR to add your organization](https://github.com/fluxcd/website/tree/main/adopters#readme) to the list!

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
  # can be: kubernetes, istio, linkerd, appmesh, nginx, skipper, contour, gloo, supergloo, traefik, osm
  # for SMI TrafficSplit can be: smi:v1alpha1, smi:v1alpha2, smi:v1alpha3
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
      # custom metric check
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

**Service Mesh**

| Feature                                    | App Mesh           | Istio              | Linkerd            | Open Service Mesh  | SMI                | Kubernetes CNI     |
| ------------------------------------------ | ------------------ | ------------------ | ------------------ | ------------------ | ------------------ | ------------------ |
| Canary deployments (weighted traffic)      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| A/B testing (headers and cookies routing)  | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: |
| Blue/Green deployments (traffic switch)    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Blue/Green deployments (traffic mirroring) | :heavy_minus_sign: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: |
| Webhooks (acceptance/load testing)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Manual gating (approve/pause/resume)       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (L7 metric)     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: |
| Request duration check (L7 metric)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: |
| Custom metric checks                       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |

For other SMI compatible service mesh solutions like Consul Connect or Nginx Service Mesh,
[Prometheus MetricTemplates](https://docs.flagger.app/usage/metrics#prometheus) can be used to implement
the request success rate and request duration checks.

**Ingress**

| Feature                                    | Contour            | Gloo               | NGINX              | Skipper            | Traefik            |
| ------------------------------------------ | ------------------ | ------------------ | ------------------ | ------------------ | ------------------ |
| Canary deployments (weighted traffic)      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| A/B testing (headers and cookies routing)  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: |
| Blue/Green deployments (traffic switch)    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Webhooks (acceptance/load testing)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Manual gating (approve/pause/resume)       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (L7 metric)     | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_check_mark: |
| Request duration check (L7 metric)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_check_mark: |
| Custom metric checks                       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |

### Roadmap

#### [GitOps Toolkit](https://github.com/fluxcd/flux2) compatibility

* Migrate Flagger to Kubernetes controller-runtime and [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
* Make the Canary status compatible with [kstatus](https://github.com/kubernetes-sigs/cli-utils)
* Make Flagger emit Kubernetes events compatible with Flux v2 notification API
* Integrate Flagger into Flux v2 as the progressive delivery component

#### Integrations

* Add support for Kubernetes [Ingress v2](https://github.com/kubernetes-sigs/service-apis)
* Add support for ingress controllers like HAProxy and ALB
* Add support for metrics providers like InfluxDB, Stackdriver, SignalFX

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
* Invite yourself to the [CNCF community slack](https://slack.cncf.io/)
  and join the [#flagger](https://cloud-native.slack.com/messages/flagger/) channel.
* Check out the **[Flux events calendar](https://fluxcd.io/#calendar)**, both with upcoming talks, events and meetings you can attend.
* Or view the **[Flux resources section](https://fluxcd.io/resources)** with past events videos you can watch.
* File an [issue](https://github.com/fluxcd/flagger/issues/new).

Your feedback is always welcome!
