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
  details: Shift and route traffic between app versions using a service mesh like Istio, Linkerd, OSM or AWS App Mesh. Or if a service mesh does not meet your needs, use an Ingress controller like Contour, Gloo, NGINX, Skipper or Traefik.
- title:  Extensible Validation
  details: Besides the builtin metrics checks, you can extend your application analysis with custom metrics and webooks for running acceptance tests, load tests, or any other custom validation. 
footer: Apache License 2.0 | Copyright © 2018-2021 The Flux authors
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
* **Canary** (progressive traffic shifting)
    * [Istio](https://docs.flagger.app/tutorials/istio-progressive-delivery),
      [Linkerd](https://docs.flagger.app/tutorials/linkerd-progressive-delivery),
      [App Mesh](https://docs.flagger.app/tutorials/appmesh-progressive-delivery),
    * [Open Service Mesh](https://docs.flagger.app/tutorials/osm-progressive-delivery),
      [Contour](https://docs.flagger.app/tutorials/contour-progressive-delivery),
      [Gloo](https://docs.flagger.app/tutorials/gloo-progressive-delivery),
      [NGINX](https://docs.flagger.app/tutorials/nginx-progressive-delivery),
      [Skipper](https://docs.flagger.app/tutorials/skipper-progressive-delivery)
      [Traefik](https://docs.flagger.app/tutorials/traefik-progressive-delivery)
      
* **A/B Testing** (HTTP headers and cookies traffic routing)
    * [Istio](https://docs.flagger.app/tutorials/istio-ab-testing),
      [App Mesh](https://docs.flagger.app/tutorials/appmesh-progressive-delivery#a-b-testing),
      [Contour](https://docs.flagger.app/tutorials/contour-progressive-delivery#a-b-testing),
      [NGINX](https://docs.flagger.app/tutorials/nginx-progressive-delivery#a-b-testing)
* **Blue/Green** (traffic switching and mirroring)
    * [Kubernetes CNI](https://docs.flagger.app/tutorials/kubernetes-blue-green),
      [Istio](https://docs.flagger.app/tutorials/istio-progressive-delivery#traffic-mirroring),
      Linkerd, App Mesh, OSM, Contour, Gloo, NGINX, Skipper, Traefik 

Flagger's application analysis can be extended with metric queries targeting Prometheus, Datadog,
CloudWatch, New Relic, Graphite, Dynatrace, InfluxDB and Google Cloud Monitoring (Stackdriver).

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

[Weaveworks](https://www.weave.works/technologies/gitops/)
:::

GitOps tutorials:
* [Progressive Delivery for Istio with Flagger and Flux](https://github.com/stefanprodan/gitops-istio)
* [Progressive Delivery for AWS App Mesh with Flagger and Flux](https://eks.handson.flagger.dev)
* [Progressive Delivery for Linkerd and Contour with Flagger and Flux](https://github.com/stefanprodan/gitops-linkerd)

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
