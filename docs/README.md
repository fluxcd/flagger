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
  details: Shift and route traffic between app versions using a service mesh like Istio, Linkerd or AWS App Mesh. Or if a service mesh does not meet your needs, use an Ingress controller like NGINX or Gloo.
- title:  Extensible Validation
  details: Besides the builtin metrics checks, you can extend your application analysis with custom Prometheus metrics and webooks for running acceptance tests, load tests, or any other custom validation. 
footer: Apache License 2.0 | Copyright © 2019 Weaveworks
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
    * [Istio](https://docs.flagger.app/usage/progressive-delivery),
      [Linkerd](https://docs.flagger.app/usage/linkerd-progressive-delivery),
      [App Mesh](https://docs.flagger.app/usage/appmesh-progressive-delivery),
      [NGINX](https://docs.flagger.app/usage/nginx-progressive-delivery),
      [Gloo](https://docs.flagger.app/usage/gloo-progressive-delivery)
* **A/B Testing** (HTTP headers and cookies traffic routing)
    * [Istio](https://docs.flagger.app/usage/ab-testing),
      [NGINX](https://docs.flagger.app/usage/nginx-progressive-delivery#a-b-testing)
* **Blue/Green** (traffic switching)
    * [Kubernetes CNI](https://docs.flagger.app/usage/blue-green)

Flagger can be configured to send notifications to a
[Slack](https://docs.flagger.app/usage/alerting#slack) or
[Microsoft Teams](https://docs.flagger.app/usage/alerting#microsoft-teams) channel.
It will post messages when a deployment has been initialised,
when a new revision has been detected and if the canary analysis failed or succeeded.

## GitOps

![GtiOps with Flagger and FluxCD](/flagger-gitops.png)

You can build fully automated GitOps pipelines for canary deployments with Flagger and
[FluxCD](https://github.com/fluxcd/flux) (CNCF sandbox project).

::: tip GitOps

GitOps is a way to do Kubernetes cluster management and application delivery.
It works by using Git as a single source of truth for declarative infrastructure and applications.
With Git at the center of your delivery pipelines, developers can make pull requests
to accelerate and simplify application deployments and operations tasks to Kubernetes.

[Weaveworks](https://www.weave.works/technologies/gitops/)
:::

GitOps tutorials:
* [Progressive Delivery for Istio with Flagger and Flux](https://github.com/stefanprodan/gitops-istio)
* [Canaries with Helm charts and GitOps](https://docs.flagger.app/tutorials/canary-helm-gitops)
* [Progressive Delivery for Linkerd with Flagger, FluxCD and Helm v3](https://helm.workshop.flagger.dev)

## Getting Help

If you have any questions about Flagger and progressive delivery:

* Read the Flagger [docs](https://docs.flagger.app).
* Invite yourself to the [Weave community slack](https://slack.weave.works/)
  and join the [#flagger](https://weave-community.slack.com/messages/flagger/) channel.
* Join the [Weave User Group](https://www.meetup.com/pro/Weave/) and get invited to online talks,
  hands-on training and meetups in your area.
* File an [issue](https://github.com/weaveworks/flagger/issues/new).

Your feedback is always welcome!
