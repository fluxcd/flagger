---
title: Flagger
home: true
heroText: Flagger
tagline: Progressive Delivery Operator for Kubernetes
actionText: Get Started →
actionLink: https://docs.flagger.app
features:
- title: Safer Releases
  details: Reduce the risk of introducing a new software version in production by gradually shifting traffic to the new version while measuring metrics like HTTP/gRPC request success rate and latency.
- title: Flexible Traffic Routing
  details: Shift and route traffic between app versions automatically using an ingress controller or a service mesh compatible with Kubernetes Gateway API.
- title:  Extensible Validation
  details: Besides the builtin metrics checks, you can extend your application analysis with custom metrics and webhooks for running acceptance tests, load tests, or any other custom validation. 
footer: Apache License 2.0 | Copyright © 2018-2025 The Flux authors
---

## Progressive Delivery

Flagger was designed to give developers confidence in automating production releases with progressive delivery techniques. 

::: tip Canary release

A benefit of using canary releases is the ability to do capacity testing of the new version in a production environment
with a safe rollback strategy if issues are found. By slowly ramping up the load, you can monitor and capture metrics
about how the new version impacts the production environment.

[Martin Fowler](https://martinfowler.com/bliki/CanaryRelease.html)
:::

Flagger can run automated application analysis, testing, promotion and rollback for the following deployment strategies:
* **Canary** (progressive traffic shifting with session affinity)
    * [Istio](https://docs.flagger.app/tutorials/istio-progressive-delivery),
      [Linkerd](https://docs.flagger.app/tutorials/linkerd-progressive-delivery),
      [Kuma Service Mesh](https://docs.flagger.app/tutorials/kuma-progressive-delivery),
      [Gateway API](https://docs.flagger.app/tutorials/gatewayapi-progressive-delivery)
    * [Contour](https://docs.flagger.app/tutorials/contour-progressive-delivery),
      [Gloo](https://docs.flagger.app/tutorials/gloo-progressive-delivery),
      [NGINX](https://docs.flagger.app/tutorials/nginx-progressive-delivery),
      [Skipper](https://docs.flagger.app/tutorials/skipper-progressive-delivery),
      [Traefik](https://docs.flagger.app/tutorials/traefik-progressive-delivery),
      [Apache APISIX](https://docs.flagger.app/tutorials/apisix-progressive-delivery),
      [Knative](https://docs.flagger.app/tutorials/knative-progressive-delivery)
* **A/B Testing** (HTTP headers and cookies traffic routing)
    * [Istio](https://docs.flagger.app/tutorials/istio-ab-testing),
      [Gateway API](https://docs.flagger.app/tutorials/gatewayapi-progressive-delivery#a-b-testing),
      [Contour](https://docs.flagger.app/tutorials/contour-progressive-delivery#a-b-testing),
      [NGINX](https://docs.flagger.app/tutorials/nginx-progressive-delivery#a-b-testing)
* **Blue/Green** (traffic switching and mirroring)
    * [Kubernetes CNI](https://docs.flagger.app/tutorials/kubernetes-blue-green),
      [Istio](https://docs.flagger.app/tutorials/istio-progressive-delivery#traffic-mirroring),
      Linkerd, Kuma, Contour, Gloo, NGINX, Skipper, Traefik, Apache 

Flagger's application analysis can be extended with metric queries targeting Prometheus, Datadog,
CloudWatch, New Relic, Graphite, Dynatrace, InfluxDB and Google Cloud Monitoring.

Flagger can be configured to [send notifications](https://docs.flagger.app/usage/alerting) to
Slack, Microsoft Teams, Discord and Rocket.
It will post messages when a deployment has been initialised,
when a new revision has been detected and if the canary analysis failed or succeeded.

## GitOps

![GitOps with Flagger and Flux](/flagger-gitops.png)

You can build fully automated GitOps pipelines for canary deployments with Flagger and
[Flux](https://github.com/fluxcd/flux2).

::: tip GitOps

GitOps is a way to do Kubernetes cluster management and application delivery.
It works by using Git as a single source of truth for declarative infrastructure and applications.
With Git at the center of your delivery pipelines, developers can make pull requests
to accelerate and simplify application deployments and operations tasks to Kubernetes.

:::

## Getting Help

If you have any questions about Flagger and progressive delivery:

* Read the Flagger [docs](https://docs.flagger.app).
* Invite yourself to the [CNCF community slack](https://slack.cncf.io/)
  and join the [#flagger](https://cloud-native.slack.com/messages/flagger/) channel.
* Check out the [Flux talks section](https://fluxcd.io/community/#talks) and to see a list of online talks,
  hands-on training and meetups.
* File an [issue](https://github.com/fluxcd/flagger/issues/new).

Your feedback is always welcome!

## License

Flagger is [Apache 2.0](https://raw.githubusercontent.com/fluxcd/flagger/main/LICENSE)
licensed and accepts contributions via GitHub pull requests.

Flagger was initially developed in 2018 at Weaveworks by Stefan Prodan.
In 2020 Flagger became a [Cloud Native Computing Foundation](https://cncf.io/) project,
part of [Flux](https://fluxcd.io) family of GitOps tools.

[![CNCF](/cncf.png)](https://cncf.io/)
