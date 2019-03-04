# Changelog

All notable changes to this project are documented in this file.

## Unreleased

#### Features

- Allow headers to be appended to HTTP requests [#70](https://github.com/stefanprodan/flagger/pull/70)

## 0.7.0 (2019-02-28)

Adds support for custom metric checks, HTTP timeouts and HTTP retries

#### Features

- Allow custom promql queries in the canary analysis spec [#60](https://github.com/stefanprodan/flagger/pull/60)
- Add HTTP timeout and retries to canary service spec [#62](https://github.com/stefanprodan/flagger/pull/62)

## 0.6.0 (2019-02-25)

Allows for [HTTPMatchRequests](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#HTTPMatchRequest) 
and [HTTPRewrite](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#HTTPRewrite) 
to be customized in the service spec of the canary custom resource.

#### Features

- Add HTTP match conditions and URI rewrite to the canary service spec [#55](https://github.com/stefanprodan/flagger/pull/55)
- Update virtual service when the canary service spec changes 
    [#54](https://github.com/stefanprodan/flagger/pull/54)
    [#51](https://github.com/stefanprodan/flagger/pull/51)

#### Improvements 

- Run e2e testing on [Kubernetes Kind](https://github.com/kubernetes-sigs/kind) for canary promotion 
    [#53](https://github.com/stefanprodan/flagger/pull/53)

## 0.5.1 (2019-02-14)

Allows skipping the analysis phase to ship changes directly to production

#### Features

- Add option to skip the canary analysis [#46](https://github.com/stefanprodan/flagger/pull/46)

#### Fixes

- Reject deployment if the pod label selector doesn't match `app: <DEPLOYMENT_NAME>` [#43](https://github.com/stefanprodan/flagger/pull/43)

## 0.5.0 (2019-01-30)

Track changes in ConfigMaps and Secrets [#37](https://github.com/stefanprodan/flagger/pull/37)

#### Features

- Promote configmaps and secrets changes from canary to primary
- Detect changes in configmaps and/or secrets and (re)start canary analysis
- Add configs checksum to Canary CRD status
- Create primary configmaps and secrets at bootstrap
- Scan canary volumes and containers for configmaps and secrets

#### Fixes

- Copy deployment labels from canary to primary at bootstrap and promotion

## 0.4.1 (2019-01-24)

Load testing webhook [#35](https://github.com/stefanprodan/flagger/pull/35)

#### Features

- Add the load tester chart to Flagger Helm repository
- Implement a load test runner based on [rakyll/hey](https://github.com/rakyll/hey)
- Log warning when no values are found for Istio metric due to lack of traffic

#### Fixes

- Run wekbooks before the metrics checks to avoid failures when using a load tester

## 0.4.0 (2019-01-18)

Restart canary analysis if revision changes [#31](https://github.com/stefanprodan/flagger/pull/31)

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

Configurable canary analysis duration [#20](https://github.com/stefanprodan/flagger/pull/20)

#### Breaking changes

- Helm chart: flag `controlLoopInterval` has been removed

#### Features

- CRD: canaries.flagger.app v1alpha3
- Schedule canary analysis independently based on `canaryAnalysis.interval`
- Add analysis interval to Canary CRD (defaults to one minute)
- Make autoscaler (HPA) reference optional

## 0.2.0 (2019-01-04)

Webhooks [#18](https://github.com/stefanprodan/flagger/pull/18)

#### Features

- CRD: canaries.flagger.app v1alpha2
- Implement canary external checks based on webhooks HTTP POST calls
- Add webhooks to Canary CRD
- Move docs to gitbook [docs.flagger.app](https://docs.flagger.app)

## 0.1.2 (2018-12-06)

Improve Slack notifications [#14](https://github.com/stefanprodan/flagger/pull/14)

#### Features

- Add canary analysis metadata to init and start Slack messages
- Add rollback reason to failed canary Slack messages

## 0.1.1 (2018-11-28)

Canary progress deadline [#10](https://github.com/stefanprodan/flagger/pull/10)

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