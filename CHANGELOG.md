# Changelog

All notable changes to this project are documented in this file.

## 1.0.0-rc.4 (2020-04-03) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

**Breaking change**: the minimum supported version of Kubernetes is v1.14.0.

#### Features

- Implement NGINX Ingress header regex matching
    [#546](https://github.com/weaveworks/flagger/pull/546)

#### Improvements

- pkg/router: update ingress API to networking.k8s.io/v1beta1
    [#534](https://github.com/weaveworks/flagger/pull/534)
- loadtester: add return cmd output option
    [#535](https://github.com/weaveworks/flagger/pull/535)
- refactoring: finalizer error handling and unit testing
    [#531](https://github.com/weaveworks/flagger/pull/535)
    [#530](https://github.com/weaveworks/flagger/pull/530)
- chart: add finalizers to RBAC rules for OpenShift
    [#537](https://github.com/weaveworks/flagger/pull/537)
- chart: allow security context to be disabled on OpenShift
    [#543](https://github.com/weaveworks/flagger/pull/543)
- chart: add annotations for service account 
    [#521](https://github.com/weaveworks/flagger/pull/521)
- docs: Add Prometheus Operator tutorial
    [#524](https://github.com/weaveworks/flagger/pull/524)

#### Fixes

- pkg/controller: avoid status conflicts on initialization
    [#544](https://github.com/weaveworks/flagger/pull/544)
- pkg/canary: fix status retry
    [#541](https://github.com/weaveworks/flagger/pull/541)
- loadtester: fix timeout errors
    [#539](https://github.com/weaveworks/flagger/pull/539)
- pkg/canary/daemonset: fix readiness check
    [#529](https://github.com/weaveworks/flagger/pull/529)
- logs: reduce log verbosity and fix typos
    [#540](https://github.com/weaveworks/flagger/pull/540)
    [#526](https://github.com/weaveworks/flagger/pull/526)


## 1.0.0-rc.3 (2020-03-23) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

#### Features

- Add opt-in finalizers to revert Flagger's mutations on deletion of a canary
    [#495](https://github.com/weaveworks/flagger/pull/495)

#### Improvements

- e2e: update end-to-end tests to Contour 1.3.0 and Gloo 1.3.14
    [#519](https://github.com/weaveworks/flagger/pull/519)
- build: update Kubernetes packages to 1.17.4
    [#516](https://github.com/weaveworks/flagger/pull/516)

#### Fixes

- Preserve node ports on service reconciliation
    [#514](https://github.com/weaveworks/flagger/pull/514)

## 1.0.0-rc.2 (2020-03-19) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

#### Features

- Make mirror percentage configurable when using Istio traffic shadowing
    [#492](https://github.com/weaveworks/flagger/pull/455)
- Add support for running Concord tests with loadtester webhooks
    [#507](https://github.com/weaveworks/flagger/pull/507)

#### Improvements

- docs: add Istio telemetry v2 upgrade guide
    [#486](https://github.com/weaveworks/flagger/pull/486),
    update A/B testing tutorial for Istio 1.5
    [#502](https://github.com/weaveworks/flagger/pull/502),
    add how to retry a failed release to FAQ
    [#494](https://github.com/weaveworks/flagger/pull/494)
- e2e: update end-to-end tests to
    Istio 1.5 [#447](https://github.com/weaveworks/flagger/pull/447) and
    NGINX Ingress 0.30
    [#489](https://github.com/weaveworks/flagger/pull/489)
    [#511](https://github.com/weaveworks/flagger/pull/511)
- refactoring:
    error handling [#480](https://github.com/weaveworks/flagger/pull/480),
    scheduler [#484](https://github.com/weaveworks/flagger/pull/484) and 
    unit tests [#475](https://github.com/weaveworks/flagger/pull/475)
- chart: add the log level configuration to Flagger helm chart
    [#506](https://github.com/weaveworks/flagger/pull/506)

#### Fixes

- Fix nil pointer for the global notifiers [#504](https://github.com/weaveworks/flagger/pull/504)

## 1.0.0-rc.1 (2020-03-03) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

Two new resources were added to the API: `MetricTemplate` and `AlertProvider`.
The analysis can reference [metric templates](https://docs.flagger.app//usage/metrics#custom-metrics)
to query Prometheus, Datadog and AWS CloudWatch.
[Alerting](https://docs.flagger.app/v/master/usage/alerting#canary-configuration) can be configured on a per
canary basis for Slack, MS Teams, Discord and Rocket.

#### Features

- Implement metric templates for Prometheus [#419](https://github.com/weaveworks/flagger/pull/419),
    Datadog [#460](https://github.com/weaveworks/flagger/pull/460) and
    CloudWatch [#464](https://github.com/weaveworks/flagger/pull/464)
- Implement metric range validation [#424](https://github.com/weaveworks/flagger/pull/424)
- Add support for targeting DaemonSets [#455](https://github.com/weaveworks/flagger/pull/455)
- Implement canary alerts and alert providers (Slack, MS Teams, Discord and Rocket)
    [#429](https://github.com/weaveworks/flagger/pull/429)

#### Improvements

- Add support for Istio multi-cluster
    [#447](https://github.com/weaveworks/flagger/pull/447) [#450](https://github.com/weaveworks/flagger/pull/450)
- Extend Istio traffic policy [#441](https://github.com/weaveworks/flagger/pull/441),
    add support for header operations [#442](https://github.com/weaveworks/flagger/pull/442) and 
    set ingress destination port when multiple ports are discovered [#436](https://github.com/weaveworks/flagger/pull/436)
- Add support for rollback gating [#449](https://github.com/weaveworks/flagger/pull/449)
- Allow disabling ConfigMaps and Secrets tracking [#425](https://github.com/weaveworks/flagger/pull/425)

#### Fixes

- Fix spec changes detection [#446](https://github.com/weaveworks/flagger/pull/446)
- Track projected ConfigMaps and Secrets [#433](https://github.com/weaveworks/flagger/pull/433)

## 0.23.0 (2020-02-06) 

Adds support for service name configuration and rollback webhook

#### Features

- Implement service name override [#416](https://github.com/weaveworks/flagger/pull/416)
- Add support for gated rollback [#420](https://github.com/weaveworks/flagger/pull/420)

## 0.22.0 (2020-01-16) 

Adds event dispatching through webhooks

#### Features

- Implement event dispatching webhook [#409](https://github.com/weaveworks/flagger/pull/409)
- Add general purpose event webhook [#401](https://github.com/weaveworks/flagger/pull/401)

#### Improvements

- Update Contour to v1.1 and add Linkerd header [#411](https://github.com/weaveworks/flagger/pull/411)
- Update Istio e2e to v1.4.3 [#407](https://github.com/weaveworks/flagger/pull/407)
- Update Kubernetes packages to 1.17 [#406](https://github.com/weaveworks/flagger/pull/406)

## 0.21.0 (2020-01-06) 

Adds support for Contour ingress controller

#### Features

- Add support for Contour ingress controller [#397](https://github.com/weaveworks/flagger/pull/397)
- Add support for Envoy managed by Crossover via SMI [#386](https://github.com/weaveworks/flagger/pull/386)
- Extend canary target ref to Kubernetes Service kind [#372](https://github.com/weaveworks/flagger/pull/372)

#### Improvements 

- Add Prometheus operator PodMonitor template to Helm chart [#399](https://github.com/weaveworks/flagger/pull/399)
- Update e2e tests to Kubernetes v1.16 [#390](https://github.com/weaveworks/flagger/pull/390)

## 0.20.4 (2019-12-03) 

Adds support for taking over a running deployment without disruption

#### Improvements 

- Add initialization phase to Kubernetes router [#384](https://github.com/weaveworks/flagger/pull/384)
- Add canary controller interface and Kubernetes deployment kind implementation [#378](https://github.com/weaveworks/flagger/pull/378)

#### Fixes

- Skip primary check on skip analysis [#380](https://github.com/weaveworks/flagger/pull/380)

## 0.20.3 (2019-11-13) 

Adds wrk to load tester tools and the App Mesh gateway chart to Flagger Helm repository

#### Improvements 

- Add wrk to load tester tools [#368](https://github.com/weaveworks/flagger/pull/368)
- Add App Mesh gateway chart [#365](https://github.com/weaveworks/flagger/pull/365)

## 0.20.2 (2019-11-07) 

Adds support for exposing canaries outside the cluster using App Mesh Gateway annotations 

#### Improvements 

- Expose canaries on public domains with App Mesh Gateway [#358](https://github.com/weaveworks/flagger/pull/358)

#### Fixes

- Use the specified replicas when scaling up the canary [#363](https://github.com/weaveworks/flagger/pull/363)

## 0.20.1 (2019-11-03) 

Fixes promql execution and updates the load testing tools

#### Improvements 

- Update load tester Helm tools [#8349dd1](https://github.com/weaveworks/flagger/commit/8349dd1cda59a741c7bed9a0f67c0fc0fbff4635)
- e2e testing: update providers [#346](https://github.com/weaveworks/flagger/pull/346)

#### Fixes

- Fix Prometheus query escape [#353](https://github.com/weaveworks/flagger/pull/353)
- Updating hey release link [#350](https://github.com/weaveworks/flagger/pull/350)

## 0.20.0 (2019-10-21) 

Adds support for [A/B Testing](https://docs.flagger.app/usage/progressive-delivery#traffic-mirroring)
and retry policies when using App Mesh

#### Features

- Implement App Mesh A/B testing based on HTTP headers match conditions [#340](https://github.com/weaveworks/flagger/pull/340)
- Implement App Mesh HTTP retry policy [#338](https://github.com/weaveworks/flagger/pull/338)
- Implement metrics server override [#342](https://github.com/weaveworks/flagger/pull/342)

#### Improvements 

- Add the app/name label to services and primary deployment [#333](https://github.com/weaveworks/flagger/pull/333)
- Allow setting Slack and Teams URLs with env vars [#334](https://github.com/weaveworks/flagger/pull/334)
- Refactor Gloo integration [#344](https://github.com/weaveworks/flagger/pull/344)

#### Fixes

- Generate unique names for App Mesh virtual routers and routes [#336](https://github.com/weaveworks/flagger/pull/336)

## 0.19.0 (2019-10-08) 

Adds support for canary and blue/green [traffic mirroring](https://docs.flagger.app/usage/progressive-delivery#traffic-mirroring)

#### Features

- Add traffic mirroring for Istio service mesh [#311](https://github.com/weaveworks/flagger/pull/311)
- Implement canary service target port [#327](https://github.com/weaveworks/flagger/pull/327)

#### Improvements 

- Allow gRPC protocol for App Mesh [#325](https://github.com/weaveworks/flagger/pull/325)
- Enforce blue/green when using Kubernetes networking [#326](https://github.com/weaveworks/flagger/pull/326)

#### Fixes

- Fix port discovery diff [#324](https://github.com/weaveworks/flagger/pull/324)
- Helm chart: Enable Prometheus scraping of Flagger metrics
    [#2141d88](https://github.com/weaveworks/flagger/commit/2141d88ce1cc6be220dab34171c215a334ecde24)

## 0.18.6 (2019-10-03) 

Adds support for App Mesh conformance tests and latency metric checks

#### Improvements 

- Add support for acceptance testing when using App Mesh [#322](https://github.com/weaveworks/flagger/pull/322)
- Add Kustomize installer for App Mesh [#310](https://github.com/weaveworks/flagger/pull/310)
- Update Linkerd to v2.5.0 and Prometheus to v2.12.0 [#323](https://github.com/weaveworks/flagger/pull/323)

#### Fixes

- Fix slack/teams notification fields mapping [#318](https://github.com/weaveworks/flagger/pull/318)

## 0.18.5 (2019-10-02) 

Adds support for [confirm-promotion](https://docs.flagger.app/how-it-works#webhooks)
webhooks and blue/green deployments when using a service mesh

#### Features

- Implement confirm-promotion hook [#307](https://github.com/weaveworks/flagger/pull/307)
- Implement B/G for service mesh providers [#305](https://github.com/weaveworks/flagger/pull/305)

#### Improvements 

- Canary promotion improvements to avoid dropping in-flight requests [#310](https://github.com/weaveworks/flagger/pull/310)
- Update end-to-end tests to Kubernetes v1.15.3 and Istio 1.3.0 [#306](https://github.com/weaveworks/flagger/pull/306) 

#### Fixes

- Skip primary check for App Mesh [#315](https://github.com/weaveworks/flagger/pull/315)

## 0.18.4 (2019-09-08) 

Adds support for NGINX custom annotations and Helm v3 acceptance testing

#### Features

- Add annotations prefix for NGINX ingresses [#293](https://github.com/weaveworks/flagger/pull/293)
- Add wide columns in CRD [#289](https://github.com/weaveworks/flagger/pull/289)
- loadtester: implement Helm v3 test command [#296](https://github.com/weaveworks/flagger/pull/296)
- loadtester: add gRPC health check to load tester image [#295](https://github.com/weaveworks/flagger/pull/295)

#### Fixes

- loadtester: fix tests error logging [#286](https://github.com/weaveworks/flagger/pull/286)

## 0.18.3 (2019-08-22) 

Adds support for tillerless helm tests and protobuf health checking

#### Features

- loadtester: add support for tillerless helm [#280](https://github.com/weaveworks/flagger/pull/280)
- loadtester: add support for protobuf health checking [#280](https://github.com/weaveworks/flagger/pull/280)

#### Improvements 

- Set HTTP listeners for AppMesh virtual routers [#272](https://github.com/weaveworks/flagger/pull/272)

#### Fixes

- Add missing fields to CRD validation spec [#271](https://github.com/weaveworks/flagger/pull/271)
- Fix App Mesh backends validation in CRD [#281](https://github.com/weaveworks/flagger/pull/281)

## 0.18.2 (2019-08-05) 

Fixes multi-port support for Istio

#### Fixes

- Fix port discovery for multiple port services [#267](https://github.com/weaveworks/flagger/pull/267)

#### Improvements 

- Update e2e testing to Istio v1.2.3, Gloo v0.18.8 and NGINX ingress chart v1.12.1 [#268](https://github.com/weaveworks/flagger/pull/268)

## 0.18.1 (2019-07-30) 

Fixes Blue/Green style deployments for Kubernetes and Linkerd providers

#### Fixes

- Fix Blue/Green metrics provider and add e2e tests [#261](https://github.com/weaveworks/flagger/pull/261)

## 0.18.0 (2019-07-29) 

Adds support for [manual gating](https://docs.flagger.app/how-it-works#manual-gating) and pausing/resuming an ongoing analysis

#### Features

- Implement confirm rollout gate, hook and API [#251](https://github.com/weaveworks/flagger/pull/251)

#### Improvements 

- Refactor canary change detection and status [#240](https://github.com/weaveworks/flagger/pull/240)
- Implement finalising state [#257](https://github.com/weaveworks/flagger/pull/257)
- Add gRPC load testing tool [#248](https://github.com/weaveworks/flagger/pull/248)

#### Breaking changes

- Due to the status sub-resource changes in [#240](https://github.com/weaveworks/flagger/pull/240),
    when upgrading Flagger the canaries status phase will be reset to `Initialized`
- Upgrading Flagger with Helm will fail due to Helm poor support of CRDs,
    see [workaround](https://github.com/weaveworks/flagger/issues/223)

## 0.17.0 (2019-07-08) 

Adds support for Linkerd (SMI Traffic Split API), MS Teams notifications and HA mode with leader election

#### Features

- Add Linkerd support [#230](https://github.com/weaveworks/flagger/pull/230)
- Implement MS Teams notifications [#235](https://github.com/weaveworks/flagger/pull/235)
- Implement leader election [#236](https://github.com/weaveworks/flagger/pull/236)

#### Improvements 

- Add [Kustomize](https://docs.flagger.app/install/flagger-install-on-kubernetes#install-flagger-with-kustomize)
    installer [#232](https://github.com/weaveworks/flagger/pull/232)
- Add Pod Security Policy to Helm chart [#234](https://github.com/weaveworks/flagger/pull/234)

## 0.16.0 (2019-06-23) 

Adds support for running [Blue/Green deployments](https://docs.flagger.app/usage/blue-green)
without a service mesh or ingress controller

#### Features

- Allow blue/green deployments without a service mesh provider [#211](https://github.com/weaveworks/flagger/pull/211)
- Add the service mesh provider to the canary spec [#217](https://github.com/weaveworks/flagger/pull/217)
- Allow multi-port services and implement port discovery [#207](https://github.com/weaveworks/flagger/pull/207)

#### Improvements 

- Add [FAQ page](https://docs.flagger.app/faq) to docs website
- Switch to go modules in CI [#218](https://github.com/weaveworks/flagger/pull/218)
- Update e2e testing to Kubernetes Kind 0.3.0 and Istio 1.2.0

#### Fixes

- Update the primary HPA on canary promotion [#216](https://github.com/weaveworks/flagger/pull/216)

## 0.15.0 (2019-06-12) 

Adds support for customising the Istio [traffic policy](https://docs.flagger.app/how-it-works#istio-routing) in the canary service spec

#### Features

- Generate Istio destination rules and allow traffic policy customisation [#200](https://github.com/weaveworks/flagger/pull/200)

#### Improvements 

-  Update Kubernetes packages to 1.14 and use go modules instead of dep [#202](https://github.com/weaveworks/flagger/pull/202) 

## 0.14.1 (2019-06-05) 

Adds support for running [acceptance/integration tests](https://docs.flagger.app/how-it-works#integration-testing)
with Helm test or Bash Bats using pre-rollout hooks

#### Features

- Implement Helm and Bash pre-rollout hooks [#196](https://github.com/weaveworks/flagger/pull/196)

#### Fixes

- Fix promoting canary when max weight is not a multiple of step [#190](https://github.com/weaveworks/flagger/pull/190)
- Add ability to set Prometheus url with custom path without trailing '/' [#197](https://github.com/weaveworks/flagger/pull/197)

## 0.14.0 (2019-05-21) 

Adds support for Service Mesh Interface and [Gloo](https://docs.flagger.app/usage/gloo-progressive-delivery) ingress controller

#### Features

- Add support for SMI (Istio weighted traffic) [#180](https://github.com/weaveworks/flagger/pull/180)
- Add support for Gloo ingress controller (weighted traffic) [#179](https://github.com/weaveworks/flagger/pull/179)

## 0.13.2 (2019-04-11) 

Fixes for Jenkins X deployments (prevent the jx GC from removing the primary instance)

#### Fixes

- Do not copy labels from canary to primary deployment [#178](https://github.com/weaveworks/flagger/pull/178)

#### Improvements 

- Add NGINX ingress controller e2e and unit tests [#176](https://github.com/weaveworks/flagger/pull/176) 

## 0.13.1 (2019-04-09) 

Fixes for custom metrics checks and NGINX Prometheus queries 

#### Fixes

- Fix promql queries for custom checks and NGINX [#174](https://github.com/weaveworks/flagger/pull/174)

## 0.13.0 (2019-04-08) 

Adds support for [NGINX](https://docs.flagger.app/usage/nginx-progressive-delivery) ingress controller

#### Features

- Add support for nginx ingress controller (weighted traffic and A/B testing) [#170](https://github.com/weaveworks/flagger/pull/170)
- Add Prometheus add-on to Flagger Helm chart for App Mesh and
    NGINX [79b3370](https://github.com/weaveworks/flagger/pull/170/commits/79b337089294a92961bc8446fd185b38c50a32df)

#### Fixes

- Fix duplicate hosts Istio error when using wildcards [#162](https://github.com/weaveworks/flagger/pull/162)

## 0.12.0 (2019-04-29) 

Adds support for [SuperGloo](https://docs.flagger.app/install/flagger-install-with-supergloo)

#### Features

- Supergloo support for canary deployment (weighted traffic) [#151](https://github.com/weaveworks/flagger/pull/151)

## 0.11.1 (2019-04-18) 

Move Flagger and the load tester container images to Docker Hub

#### Features

- Add Bash Automated Testing System support to Flagger tester for running acceptance tests as pre-rollout hooks 

## 0.11.0 (2019-04-17) 

Adds pre/post rollout [webhooks](https://docs.flagger.app/how-it-works#webhooks)

#### Features

- Add `pre-rollout` and `post-rollout` webhook types [#147](https://github.com/weaveworks/flagger/pull/147)

#### Improvements 

- Unify App Mesh and Istio builtin metric checks [#146](https://github.com/weaveworks/flagger/pull/146) 
- Make the pod selector label configurable [#148](https://github.com/weaveworks/flagger/pull/148)

#### Breaking changes

- Set default `mesh` Istio gateway only if no gateway is specified [#141](https://github.com/weaveworks/flagger/pull/141)

## 0.10.0 (2019-03-27) 

Adds support for App Mesh

#### Features

- AWS App Mesh integration
    [#107](https://github.com/weaveworks/flagger/pull/107)
    [#123](https://github.com/weaveworks/flagger/pull/123)

#### Improvements 

- Reconcile Kubernetes ClusterIP services [#122](https://github.com/weaveworks/flagger/pull/122) 
 
#### Fixes

- Preserve pod labels on canary promotion [#105](https://github.com/weaveworks/flagger/pull/105)
- Fix canary status Prometheus metric [#121](https://github.com/weaveworks/flagger/pull/121)

## 0.9.0 (2019-03-11)

Allows A/B testing scenarios where instead of weighted routing, the traffic is split between the 
primary and canary based on HTTP headers or cookies.

#### Features

- A/B testing - canary with session affinity [#88](https://github.com/weaveworks/flagger/pull/88)

#### Fixes

- Update the analysis interval when the custom resource changes [#91](https://github.com/weaveworks/flagger/pull/91)

## 0.8.0 (2019-03-06)

Adds support for CORS policy and HTTP request headers manipulation

#### Features

- CORS policy support [#83](https://github.com/weaveworks/flagger/pull/83)
- Allow headers to be appended to HTTP requests [#82](https://github.com/weaveworks/flagger/pull/82)

#### Improvements 

- Refactor the routing management 
    [#72](https://github.com/weaveworks/flagger/pull/72) 
    [#80](https://github.com/weaveworks/flagger/pull/80)
- Fine-grained RBAC [#73](https://github.com/weaveworks/flagger/pull/73)
- Add option to limit Flagger to a single namespace [#78](https://github.com/weaveworks/flagger/pull/78)

## 0.7.0 (2019-02-28)

Adds support for custom metric checks, HTTP timeouts and HTTP retries

#### Features

- Allow custom promql queries in the canary analysis spec [#60](https://github.com/weaveworks/flagger/pull/60)
- Add HTTP timeout and retries to canary service spec [#62](https://github.com/weaveworks/flagger/pull/62)

## 0.6.0 (2019-02-25)

Allows for [HTTPMatchRequests](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#HTTPMatchRequest) 
and [HTTPRewrite](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#HTTPRewrite) 
to be customized in the service spec of the canary custom resource.

#### Features

- Add HTTP match conditions and URI rewrite to the canary service spec [#55](https://github.com/weaveworks/flagger/pull/55)
- Update virtual service when the canary service spec changes 
    [#54](https://github.com/weaveworks/flagger/pull/54)
    [#51](https://github.com/weaveworks/flagger/pull/51)

#### Improvements 

- Run e2e testing on [Kubernetes Kind](https://github.com/kubernetes-sigs/kind) for canary promotion 
    [#53](https://github.com/weaveworks/flagger/pull/53)

## 0.5.1 (2019-02-14)

Allows skipping the analysis phase to ship changes directly to production

#### Features

- Add option to skip the canary analysis [#46](https://github.com/weaveworks/flagger/pull/46)

#### Fixes

- Reject deployment if the pod label selector doesn't match `app: <DEPLOYMENT_NAME>` [#43](https://github.com/weaveworks/flagger/pull/43)

## 0.5.0 (2019-01-30)

Track changes in ConfigMaps and Secrets [#37](https://github.com/weaveworks/flagger/pull/37)

#### Features

- Promote configmaps and secrets changes from canary to primary
- Detect changes in configmaps and/or secrets and (re)start canary analysis
- Add configs checksum to Canary CRD status
- Create primary configmaps and secrets at bootstrap
- Scan canary volumes and containers for configmaps and secrets

#### Fixes

- Copy deployment labels from canary to primary at bootstrap and promotion

## 0.4.1 (2019-01-24)

Load testing webhook [#35](https://github.com/weaveworks/flagger/pull/35)

#### Features

- Add the load tester chart to Flagger Helm repository
- Implement a load test runner based on [rakyll/hey](https://github.com/rakyll/hey)
- Log warning when no values are found for Istio metric due to lack of traffic

#### Fixes

- Run wekbooks before the metrics checks to avoid failures when using a load tester

## 0.4.0 (2019-01-18)

Restart canary analysis if revision changes [#31](https://github.com/weaveworks/flagger/pull/31)

#### Breaking changes

- Drop support for Kubernetes 1.10

#### Features

- Detect changes during canary analysis and reset advancement
- Add status and additional printer columns to CRD
- Add canary name and namespace to controller structured logs

#### Fixes

- Allow canary name to be different to the target name
- Check if multiple canaries have the same target and log error
- Use deep copy when updating Kubernetes objects
- Skip readiness checks if canary analysis has finished

## 0.3.0 (2019-01-11)

Configurable canary analysis duration [#20](https://github.com/weaveworks/flagger/pull/20)

#### Breaking changes

- Helm chart: flag `controlLoopInterval` has been removed

#### Features

- CRD: canaries.flagger.app v1alpha3
- Schedule canary analysis independently based on `canaryAnalysis.interval`
- Add analysis interval to Canary CRD (defaults to one minute)
- Make autoscaler (HPA) reference optional

## 0.2.0 (2019-01-04)

Webhooks [#18](https://github.com/weaveworks/flagger/pull/18)

#### Features

- CRD: canaries.flagger.app v1alpha2
- Implement canary external checks based on webhooks HTTP POST calls
- Add webhooks to Canary CRD
- Move docs to gitbook [docs.flagger.app](https://docs.flagger.app)

## 0.1.2 (2018-12-06)

Improve Slack notifications [#14](https://github.com/weaveworks/flagger/pull/14)

#### Features

- Add canary analysis metadata to init and start Slack messages
- Add rollback reason to failed canary Slack messages

## 0.1.1 (2018-11-28)

Canary progress deadline [#10](https://github.com/weaveworks/flagger/pull/10)

#### Features

- Rollback canary based on the deployment progress deadline check
- Add progress deadline to Canary CRD (defaults to 10 minutes)

## 0.1.0 (2018-11-25)

First stable release

#### Features

- CRD: canaries.flagger.app v1alpha1
- Notifications: post canary events to Slack
- Instrumentation: expose Prometheus metrics for canary status and traffic weight percentage
- Autoscaling: add HPA reference to CRD and create primary HPA at bootstrap
- Bootstrap: create primary deployment, ClusterIP services and Istio virtual service based on CRD spec


## 0.0.1 (2018-10-07)

Initial semver release

#### Features

- Implement canary rollback based on failed checks threshold
- Scale up the deployment when canary revision changes
- Add OpenAPI v3 schema validation to Canary CRD
- Use CRD status for canary state persistence
- Add Helm charts for Flagger and Grafana
- Add canary analysis Grafana dashboard 
