 # flagger

[![release](https://img.shields.io/github/release/fluxcd/flagger/all.svg)](https://github.com/fluxcd/flagger/releases)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4783/badge)](https://bestpractices.coreinfrastructure.org/projects/4783)
[![report](https://goreportcard.com/badge/github.com/fluxcd/flagger)](https://goreportcard.com/report/github.com/fluxcd/flagger)
[![FOSSA Status](https://app.fossa.com/api/projects/custom%2B162%2Fgithub.com%2Ffluxcd%2Fflagger.svg?type=shield)](https://app.fossa.com/projects/custom%2B162%2Fgithub.com%2Ffluxcd%2Fflagger?ref=badge_shield)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/flagger)](https://artifacthub.io/packages/search?repo=flagger)
[![CLOMonitor](https://img.shields.io/endpoint?url=https://clomonitor.io/api/projects/cncf/flagger/badge)](https://clomonitor.io/projects/cncf/flagger)

Flagger is a progressive delivery tool that automates the release process for applications running on Kubernetes. 
It reduces the risk of introducing a new software version in production
by gradually shifting traffic to the new version while measuring metrics and running conformance tests.

![flagger-overview](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-overview.png)

Flagger implements several deployment strategies (Canary releases, A/B testing, Blue/Green mirroring)
and integrates with various Kubernetes ingress controllers, service mesh, and monitoring solutions.

Flagger is a [Cloud Native Computing Foundation](https://cncf.io/) project
and part of the [Flux](https://fluxcd.io) family of GitOps tools.

### Documentation

Flagger documentation can be found at [fluxcd.io/flagger](https://fluxcd.io/flagger/).

* Install
  * [Flagger install on Kubernetes](https://fluxcd.io/flagger/install/flagger-install-on-kubernetes)
* Usage
  * [How it works](https://fluxcd.io/flagger/usage/how-it-works)
  * [Deployment strategies](https://fluxcd.io/flagger/usage/deployment-strategies)
  * [Metrics analysis](https://fluxcd.io/flagger/usage/metrics)
  * [Webhooks](https://fluxcd.io/flagger/usage/webhooks)
  * [Alerting](https://fluxcd.io/flagger/usage/alerting)
  * [Monitoring](https://fluxcd.io/flagger/usage/monitoring)
* Tutorials
  * [App Mesh](https://fluxcd.io/flagger/tutorials/appmesh-progressive-delivery)
  * [Istio](https://fluxcd.io/flagger/tutorials/istio-progressive-delivery)
  * [Linkerd](https://fluxcd.io/flagger/tutorials/linkerd-progressive-delivery)
  * [Open Service Mesh (OSM)](https://dfluxcd.io/flagger/tutorials/osm-progressive-delivery)
  * [Kuma Service Mesh](https://fluxcd.io/flagger/tutorials/kuma-progressive-delivery)
  * [Contour](https://fluxcd.io/flagger/tutorials/contour-progressive-delivery)
  * [Gloo](https://fluxcd.io/flagger/tutorials/gloo-progressive-delivery)
  * [NGINX Ingress](https://fluxcd.io/flagger/tutorials/nginx-progressive-delivery)
  * [Skipper](https://fluxcd.io/flagger/tutorials/skipper-progressive-delivery)
  * [Traefik](https://fluxcd.io/flagger/tutorials/traefik-progressive-delivery)
  * [Gateway API](https://fluxcd.io/flagger/tutorials/gatewayapi-progressive-delivery/)
  * [Kubernetes Blue/Green](https://fluxcd.io/flagger/tutorials/kubernetes-blue-green)

### Adopters

**Our list of production users has moved to <https://fluxcd.io/adopters/#flagger>**.

If you are using Flagger, please
[submit a PR to add your organization](https://github.com/fluxcd/website/tree/main/adopters#readme) to the list!

### Canary CRD

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services, service mesh, or ingress routes).
These objects expose the application on the mesh and drive the canary analysis and promotion.

Flagger keeps track of ConfigMaps and Secrets referenced by a Kubernetes Deployment and triggers a canary analysis if any of those objects change.
When promoting a workload in production, both code (container images) and configuration (config maps and secrets) are being synchronized.

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
    apiVersion: autoscaling/v2
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

For more details on how the canary analysis and promotion works please [read the docs](https://fluxcd.io/flagger/usage/how-it-works).

### Features

**Service Mesh**

| Feature                                    | App Mesh           | Istio              | Linkerd            | Kuma               | OSM                | Kubernetes CNI     |
|--------------------------------------------|--------------------|--------------------|--------------------|--------------------|--------------------|--------------------|
| Canary deployments (weighted traffic)      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| A/B testing (headers and cookies routing)  | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: |
| Blue/Green deployments (traffic switch)    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Blue/Green deployments (traffic mirroring) | :heavy_minus_sign: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: |
| Webhooks (acceptance/load testing)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Manual gating (approve/pause/resume)       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (L7 metric)     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| Request duration check (L7 metric)         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: |
| Custom metric checks                       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |

**Ingress**

| Feature                                   | Contour            | Gloo               | NGINX              | Skipper            | Traefik            | Apache APISIX      |
|-------------------------------------------|--------------------|--------------------|--------------------|--------------------|--------------------|--------------------|
| Canary deployments (weighted traffic)     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| A/B testing (headers and cookies routing) | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_minus_sign: | :heavy_minus_sign: |
| Blue/Green deployments (traffic switch)   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Webhooks (acceptance/load testing)        | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Manual gating (approve/pause/resume)      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (L7 metric)    | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Request duration check (L7 metric)        | :heavy_check_mark: | :heavy_check_mark: | :heavy_minus_sign: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Custom metric checks                      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |

**Networking Interface**

| Feature                                       | Gateway API        | SMI                |
|-----------------------------------------------|--------------------|--------------------|
| Canary deployments (weighted traffic)         | :heavy_check_mark: | :heavy_check_mark: |
| A/B testing (headers and cookies routing)     | :heavy_check_mark: | :heavy_minus_sign: |
| Blue/Green deployments (traffic switch)       | :heavy_check_mark: | :heavy_check_mark: |
| Blue/Green deployments (traffic mirrroring)   | :heavy_minus_sign: | :heavy_minus_sign: |
| Webhooks (acceptance/load testing)            | :heavy_check_mark: | :heavy_check_mark: |
| Manual gating (approve/pause/resume)          | :heavy_check_mark: | :heavy_check_mark: |
| Request success rate check (L7 metric)        | :heavy_minus_sign: | :heavy_minus_sign: |
| Request duration check (L7 metric)            | :heavy_minus_sign: | :heavy_minus_sign: |
| Custom metric checks                          | :heavy_check_mark: | :heavy_check_mark: |

For all [Gateway API](https://gateway-api.sigs.k8s.io/) implementations like
[Contour](https://projectcontour.io/guides/gateway-api/) or
[Istio](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/)
and [SMI](https://smi-spec.io) compatible service mesh solutions like 
[Nginx Service Mesh](https://docs.nginx.com/nginx-service-mesh/),
[Prometheus MetricTemplates](https://docs.flagger.app/usage/metrics#prometheus)
can be used to implement the request success rate and request duration checks.

### Roadmap

#### [GitOps Toolkit](https://github.com/fluxcd/flux2) compatibility

- Migrate Flagger to Kubernetes controller-runtime and [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
- Make the Canary status compatible with [kstatus](https://github.com/kubernetes-sigs/cli-utils)
- Make Flagger emit Kubernetes events compatible with Flux v2 notification API
- Integrate Flagger into Flux v2 as the progressive delivery component

#### Integrations

- Add support for ingress controllers like HAProxy, ALB, and Apache APISIX
- Add support for Knative Serving

### Contributing

Flagger is Apache 2.0 licensed and accepts contributions via GitHub pull requests.
To start contributing please read the [development guide](https://docs.flagger.app/dev/dev-guide).

When submitting bug reports please include as many details as possible:

- which Flagger version
- which Kubernetes version
- what configuration (canary, ingress and workloads definitions)
- what happened (Flagger and Proxy logs)

### Communication

Here is a list of good entry points into our community, how we stay in touch and how you can meet us as a team.

- Slack: Join in and talk to us in the `#flagger` channel on [CNCF Slack](https://slack.cncf.io/).
- Public meetings: We run weekly meetings - join one of the upcoming dev meetings from the [Flux calendar](https://fluxcd.io/#calendar).
- Blog: Stay up to date with the latest news on [the Flux blog](https://fluxcd.io/blog/).
- Mailing list: To be updated on Flux and Flagger progress regularly, please [join the flux-dev mailing list](https://lists.cncf.io/g/cncf-flux-dev).

#### Subscribing to the flux-dev calendar

To add the meetings to your e.g. Google calendar

1. visit the [Flux calendar](https://lists.cncf.io/g/cncf-flux-dev/calendar)
2. click on "Subscribe to Calendar" at the very bottom of the page
3. copy the iCalendar URL
4. open e.g. your Google calendar
5. find the "add calendar" option
6. choose "add by URL"
7. paste iCalendar URL (ends with `.ics`)
8. done
