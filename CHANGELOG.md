# Changelog

All notable changes to this project are documented in this file.

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

- Due to the status sub-resource changes in [#240](https://github.com/weaveworks/flagger/pull/240), when upgrading Flagger the canaries status phase will be reset to `Initialized`
- Upgrading Flagger with Helm will fail due to Helm poor support of CRDs, see [workaround](https://github.com/weaveworks/flagger/issues/223)

## 0.17.0 (2019-07-08) 

Adds support for Linkerd (SMI Traffic Split API), MS Teams notifications and HA mode with leader election

#### Features

- Add Linkerd support [#230](https://github.com/weaveworks/flagger/pull/230)
- Implement MS Teams notifications [#235](https://github.com/weaveworks/flagger/pull/235)
- Implement leader election [#236](https://github.com/weaveworks/flagger/pull/236)

#### Improvements 

- Add [Kustomize](https://docs.flagger.app/install/flagger-install-on-kubernetes#install-flagger-with-kustomize) installer [#232](https://github.com/weaveworks/flagger/pull/232)
- Add Pod Security Policy to Helm chart [#234](https://github.com/weaveworks/flagger/pull/234)

## 0.16.0 (2019-06-23) 

Adds support for running [Blue/Green deployments](https://docs.flagger.app/usage/blue-green) without a service mesh or ingress controller

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

Adds support for running [acceptance/integration tests](https://docs.flagger.app/how-it-works#integration-testing) with Helm test or Bash Bats using pre-rollout hooks

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
- Add Prometheus add-on to Flagger Helm chart for App Mesh and NGINX [79b3370](https://github.com/weaveworks/flagger/pull/170/commits/79b337089294a92961bc8446fd185b38c50a32df)

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