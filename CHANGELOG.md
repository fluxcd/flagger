# Changelog

All notable changes to this project are documented in this file.

## 1.32.0

**Release date:** 2023-07-14

This release adds support for suspending a Canary using `.spec.suspend`.
It also fixes a bug where the target deployment gets stuck at 0 replicas
after the Canary has been deleted.
Furthermore, the Canary API has been modified to allow specifying the
HTTPRoute port using `.service.gatewayRefs[].port`.

#### Improvements
- Helm: Add option to create service and serviceMonitor
  [#1425](https://github.com/fluxcd/flagger/pull/1425)
- Update Alpine to 3.18
  [#1426](https://github.com/fluxcd/flagger/pull/1426)
- Add `spec.suspend` to allow suspending canary
  [#1431](https://github.com/fluxcd/flagger/pull/1431)
- Add support for istio LEAST_REQUEST destination rule load balancing
  [#1439](https://github.com/fluxcd/flagger/pull/1439)
- Add gatewayRef port to Canary CRD
  [#1453](https://github.com/fluxcd/flagger/pull/1453)
- feat: Copy slowStartConfig for Gloo upstreams
  [#1455](https://github.com/fluxcd/flagger/pull/1455)
- Update Go dependencies
  [#1459](https://github.com/fluxcd/flagger/pull/1459)

#### Fixes
- Resume target scaler during finalization
  [#1429](https://github.com/fluxcd/flagger/pull/1429)
- Fix panic when annotation of ingress is empty
  [#1437](https://github.com/fluxcd/flagger/pull/1437)
- Fixing namespace of HelmRepository in installation docs
  [#1458](https://github.com/fluxcd/flagger/pull/1458)

## 1.31.0

**Release date:** 2023-05-10

⚠️  __Breaking Changes__

This release adds support for Linkerd 2.12 and later. Due to changes in Linkerd
the default namespace for Flagger's installation had to be changed from
`linkerd` to `flagger-system` and the `flagger` Deployment is now injected with
the Linkerd proxy. Furthermore, installing Flagger for Linkerd will result in
the creation of an `AuthorizationPolicy` that allows access to the Prometheus
instance in the `linkerd-viz` namespace. To upgrade your Flagger installation,
please see the below migration guide.

If you use Kustomize, then follow these steps:
* `kubectl delete -n linkerd deploy/flagger`
* `kubectl delete -n linkerd serviceaccount flagger`
* If you're on Linkerd >= 2.12, you'll need to install the SMI extension to enable
  support for `TrafficSplit`s:
  ```bash
  curl -sL https://linkerd.github.io/linkerd-smi/install | sh
  linkerd smi install | kubectl apply -f -
  ```
* `kubectl apply -k github.com/fluxcd/flagger//kustomize/linkerd`

  Note: If you're on Linkerd < 2.12, this will report an error about missing CRDs.
  It is safe to ignore this error.

If you use Helm and are on Linkerd < 2.12, then you can use `helm upgrade` to do
a regular upgrade.

If you use Helm and are on Linkerd >= 2.12, then follow these steps:
* `helm uninstall flagger -n linkerd`
* Install the Linkerd SMI extension:
  ```bash
  helm repo add l5d-smi https://linkerd.github.io/linkerd-smi
  helm install linkerd-smi l5d-smi/linkerd-smi -n linkerd-smi --create-namespace
  ```
* Install Flagger in the `flagger-system` namespace
  and create an `AuthorizationPolicy`:
  ```bash
  helm repo update flagger
  helm install flagger flagger/flagger \
  --namespace flagger-system \
  --set meshProvider=linkerd \
  --set metricsServer=http://prometheus.linkerd-viz:9090 \
  --set linkerdAuthPolicy.create=true
  ```

Furthermore, a bug which led the `confirm-rollout` webhook to be executed at
every step of the Canary instead of only being executed before the canary
Deployment is scaled up, has been fixed.

#### Improvements

- Add support for Linkerd 2.13
  [#1417](https://github.com/fluxcd/flagger/pull/1417)

#### Fixes

- Fix the loadtester install with flux documentation
  [#1384](https://github.com/fluxcd/flagger/pull/1384)
- Run `confirm-rollout` checks only before scaling up deployment
  [#1414](https://github.com/fluxcd/flagger/pull/1414)
- e2e: Remove OSM tests
  [#1423](https://github.com/fluxcd/flagger/pull/1423)

## 1.30.0

**Release date:** 2023-04-12

This release fixes a bug related to the lack of updates to the generated
object's metadata according to the metadata specified in `spec.service.apex`.
Furthermore, a bug where labels were wrongfully copied over from the canary
deployment to primary deployment when no value was provided for
`--include-label-prefix` has been fixed.
This release also makes Flagger compatible with Flux's helm-controller drift
detection.

#### Improvements

- build(deps): bump actions/cache from 3.2.5 to 3.3.1
  [#1385](https://github.com/fluxcd/flagger/pull/1385)
- helm: Added the option to supply additional volumes
  [#1393](https://github.com/fluxcd/flagger/pull/1393)
- build(deps): bump actions/setup-go from 3 to 4
  [#1394](https://github.com/fluxcd/flagger/pull/1394)
- update Kuma version and docs
  [#1402](https://github.com/fluxcd/flagger/pull/1402)
- ci: bump k8s to 1.24 and kind to 1.18 
  [#1406](https://github.com/fluxcd/flagger/pull/1406)
- Helm: Allow configuring deployment `annotations`
  [#1411](https://github.com/fluxcd/flagger/pull/1411)
- update dependencies
  [#1412](https://github.com/fluxcd/flagger/pull/1412)

#### Fixes

- Enable updates for labels and annotations
  [#1392](https://github.com/fluxcd/flagger/pull/1392)
- Update flagger-install-with-flux.md
  [#1398](https://github.com/fluxcd/flagger/pull/1398)
- avoid copying canary labels to primary on promotion
  [#1405](https://github.com/fluxcd/flagger/pull/1405)
- Disable Flux helm drift detection for managed resources
  [#1408](https://github.com/fluxcd/flagger/pull/1408)

## 1.29.0

**Release date:** 2023-02-21

This release comes with support for template variables for analysis metrics.
A canary analysis metric can reference a set of custom variables with
`.spec.analysis.metrics[].templateVariables`. For more info see the [docs](https://fluxcd.io/flagger/usage/metrics/#custom-metrics).
Furthemore, a bug related to Canary releases with session affinity has been
fixed.

#### Improvements

- update dependencies
  [#1374](https://github.com/fluxcd/flagger/pull/1374)
- build(deps): bump golang.org/x/net from 0.4.0 to 0.7.0
  [#1373](https://github.com/fluxcd/flagger/pull/1373)
- build(deps): bump fossa-contrib/fossa-action from 1 to 2
  [#1372](https://github.com/fluxcd/flagger/pull/1372)
- Allow custom affinities for flagger deployment in helm chart
  [#1371](https://github.com/fluxcd/flagger/pull/1371)
- Add namespace to namespaced resources in helm chart
  [#1370](https://github.com/fluxcd/flagger/pull/1370)
- build(deps): bump actions/cache from 3.2.4 to 3.2.5
  [#1366](https://github.com/fluxcd/flagger/pull/1366)
- build(deps): bump actions/cache from 3.2.3 to 3.2.4
  [#1362](https://github.com/fluxcd/flagger/pull/1362)
- build(deps): bump docker/build-push-action from 3 to 4
  [#1361](https://github.com/fluxcd/flagger/pull/1361)
- modify release workflow to publish rc images
  [#1359](https://github.com/fluxcd/flagger/pull/1359)
- build: Enable SBOM and SLSA Provenance
  [#1356](https://github.com/fluxcd/flagger/pull/1356)
- Add support for custom variables in metric templates
  [#1355](https://github.com/fluxcd/flagger/pull/1355)
- docs(readme.md): add additional tutorial
  [#1346](https://github.com/fluxcd/flagger/pull/1346)

#### Fixes

- use regex to match against headers in istio
  [#1364](https://github.com/fluxcd/flagger/pull/1364)

## 1.28.0

**Release date:** 2023-01-26

This release comes with support for setting a different autoscaling
configuration for the primary workload.
The `.spec.autoscalerRef.primaryScalerReplicas` is useful in the
situation where the user does not want to scale the canary workload
to the exact same size as the primary, especially when opting for a
canary deployment pattern where only a small portion of traffic is
routed to the canary workload pods.

#### Improvements

- Support for overriding primary scaler replicas
  [#1343](https://github.com/fluxcd/flagger/pull/1343)
- Allow access to Prometheus in OpenShift via SA token
  [#1338](https://github.com/fluxcd/flagger/pull/1338)
- Update Kubernetes packages to v1.26.1
  [#1352](https://github.com/fluxcd/flagger/pull/1352)

## 1.27.0

**Release date:** 2022-12-15

This release comes with support for Apachae APISIX. For more details see the
[tutorial](https://fluxcd.io/flagger/tutorials/apisix-progressive-delivery).

#### Improvements

- [apisix] Implement router interface and observer interface
  [#1281](https://github.com/fluxcd/flagger/pull/1281)
- Bump stefanprodan/helm-gh-pages from 1.6.0 to 1.7.0
  [#1326](https://github.com/fluxcd/flagger/pull/1326)
- Release loadtester v0.28.0
  [#1328](https://github.com/fluxcd/flagger/pull/1328)

#### Fixes

- Update release docs
  [#1324](https://github.com/fluxcd/flagger/pull/1324)

## 1.26.0

**Release date:** 2022-11-23

This release comes with support Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/) v1beta1.
For more details see the [Gateway API Progressive Delivery tutorial](https://docs.flagger.app/tutorials/gatewayapi-progressive-delivery).

Please note that starting with this version, the Gateway API v1alpha2 is considered deprecated
and will be removed from Flagger after 6 months.

#### Improvements:

- Updated Gateway API from v1alpha2 to v1beta1
  [#1319](https://github.com/fluxcd/flagger/pull/1319)
- Updated Gateway API docs to v1beta1
  [#1321](https://github.com/fluxcd/flagger/pull/1321)
- Update dependencies
  [#1322](https://github.com/fluxcd/flagger/pull/1322)

#### Fixes:

- docs: Add `linkerd install --crds` to Linkerd tutorial
  [#1316](https://github.com/fluxcd/flagger/pull/1316)

## 1.25.0

**Release date:** 2022-11-16

This release introduces a new deployment strategy combining Canary releases with session affinity
for Istio.

Furthermore, it contains a regression fix regarding metadata in alerts introduced in
[#1275](https://github.com/fluxcd/flagger/pull/1275)

#### Improvements:

- Add support for session affinity during weighted routing with Istio
  [#1280](https://github.com/fluxcd/flagger/pull/1280)

#### Fixes:

- Fix cluster name inclusion in alerts metadata
  [#1306](https://github.com/fluxcd/flagger/pull/1306)
- fix(faq): Update FAQ about zero downtime with correct values
  [#1302](https://github.com/fluxcd/flagger/pull/1302)

## 1.24.1

**Release date:** 2022-10-26

This release comes with a fix to Gloo routing when a custom service name id used.

In addition, the Gloo ingress end-to-end testing was updated to Gloo Helm chart v1.12.31.

#### Fixes:

- fix(gloo): Use correct route table name in case service name was overwritten
  [#1300](https://github.com/fluxcd/flagger/pull/1300)

## 1.24.0

**Release date:** 2022-10-23

Starting with this version, the Flagger release artifacts are published to
GitHub Container Registry, and they are signed with Cosign and GitHub ODIC.

OCI artifacts:

- `ghcr.io/fluxcd/flagger:<version>` multi-arch container images
- `ghcr.io/fluxcd/flagger-manifest:<version>` Kubernetes manifests
- `ghcr.io/fluxcd/charts/flagger:<version>` Helm charts

To verify an OCI artifact with Cosign:

```shell
export COSIGN_EXPERIMENTAL=1
cosign verify ghcr.io/fluxcd/flagger:1.24.0
cosign verify ghcr.io/fluxcd/flagger-manifests:1.24.0
cosign verify ghcr.io/fluxcd/charts/flagger:1.24.0
```

To deploy Flagger from its OCI artifacts the GitOps way,
please see the [Flux installation guide](docs/gitbook/install/flagger-install-with-flux.md).

#### Improvements:

- docs: Add guide on how to install Flagger with Flux OCI
  [#1294](https://github.com/fluxcd/flagger/pull/1294)
- ci: Publish signed Helm charts and manifests to GHCR
  [#1293](https://github.com/fluxcd/flagger/pull/1293)
- ci: Sign release and containers with Cosign and GitHub OIDC
  [#1292](https://github.com/fluxcd/flagger/pull/1292)
- ci: Adjust GitHub workflow permissions
  [#1286](https://github.com/fluxcd/flagger/pull/1286)
- docs: Add link to Flux governance document
  [#1286](https://github.com/fluxcd/flagger/pull/1286)

## 1.23.0

**Release date:** 2022-10-20

This release comes with support for Slack bot token authentication.

#### Improvements:

- alerts: Add support for Slack bot token authentication
  [#1270](https://github.com/fluxcd/flagger/pull/1270)
- loadtester: logCmdOutput to logger instead of stdout
  [#1267](https://github.com/fluxcd/flagger/pull/1267)
- helm: Add app.kubernetes.io/version label to chart
  [#1264](https://github.com/fluxcd/flagger/pull/1264)
- Update Go to 1.19
  [#1264](https://github.com/fluxcd/flagger/pull/1264)
- Update Kubernetes packages to v1.25.3
  [#1283](https://github.com/fluxcd/flagger/pull/1283)
- Bump Contour to v1.22 in e2e tests
  [#1282](https://github.com/fluxcd/flagger/pull/1282)

#### Fixes:

- gatewayapi: Fix reconciliation of nil hostnames
  [#1276](https://github.com/fluxcd/flagger/pull/1276)
- alerts: Include cluster name in all alerts
  [#1275](https://github.com/fluxcd/flagger/pull/1275)

## 1.22.2

**Release date:** 2022-08-29

This release fixes a bug related scaling up the canary deployment when a
reference to an autoscaler is specified.

Furthermore, it contains updates to packages used by the project, including
updates to Helm and grpc-health-probe used in the loadtester.

CVEs fixed (originating from dependencies):
* CVE-2022-37434
* CVE-2022-27191
* CVE-2021-33194
* CVE-2021-44716
* CVE-2022-29526
* CVE-2022-1996

#### Fixes:

- If HPA is set, it uses HPA minReplicas when scaling up the canary
  [#1253](https://github.com/fluxcd/flagger/pull/1253)

#### Improvements:

- Release loadtester v0.23.0
  [#1246](https://github.com/fluxcd/flagger/pull/1246)
- Add target and script to keep crds in sync
  [#1254](https://github.com/fluxcd/flagger/pull/1254)
- docs: add knative support to roadmap
  [#1258](https://github.com/fluxcd/flagger/pull/1258)
- Update dependencies
  [#1259](https://github.com/fluxcd/flagger/pull/1259)
- Release loadtester v0.24.0
  [#1261](https://github.com/fluxcd/flagger/pull/1261)

## 1.22.1

**Release date:** 2022-08-01

This minor release fixes a bug related to the use of HPA v2beta2 and updates
the KEDA ScaledObject API to include `MetricType` for `ScaleTriggers`.

Furthermore, the project has been updated to use Go 1.18 and Alpine 3.16.

#### Fixes:

- Update KEDA ScaledObject API to include MetricType for Triggers
  [#1241](https://github.com/fluxcd/flagger/pull/1241)
- Fix fallback logic for HPAv2 to v2beta2
  [#1242](https://github.com/fluxcd/flagger/pull/1242)

#### Improvements:
- Update Go to 1.18 and Alpine to 3.16
  [#1243](https://github.com/fluxcd/flagger/pull/1243)
- Clarify HPA API requirement
  [#1239](https://github.com/fluxcd/flagger/pull/1239)
- Update README
  [#1233](https://github.com/fluxcd/flagger/pull/1233)

## 1.22.0

**Release date:** 2022-07-11

This release with support for KEDA ScaledObjects as an alternative to HPAs. Check the
[tutorial](https://docs.flagger.app/tutorials/keda-scaledobject) to understand it's usage
with Flagger.

The `.spec.service.appProtocol` field can now be used to specify the [`appProtocol`](https://kubernetes.io/docs/concepts/services-networking/service/#application-protocol)
of the services that Flagger generates.

In addition, a bug related to the Contour prometheus query for when service name is overwritten
along with a bug related to a Contour `HTTPProxy` annotations have been fixed.

Furthermore, the installation guide for Alibaba ServiceMesh has been updated.

#### Improvements:

- feat: Add an optional `appProtocol` field to `spec.service`
  [#1185](https://github.com/fluxcd/flagger/pull/1185)
- Update Kubernetes packages to v1.24.1
  [#1208](https://github.com/fluxcd/flagger/pull/1208)
- charts: Add namespace parameter to parameters table
  [#1210](https://github.com/fluxcd/flagger/pull/1210)
- Introduce `ScalerReconciler` and refactor HPA reconciliation
  [#1211](https://github.com/fluxcd/flagger/pull/1211)
- e2e: Update providers and Kubernetes to v1.23
  [#1212](https://github.com/fluxcd/flagger/pull/1212)
- Add support for KEDA ScaledObjects as an auto scaler
  [#1216](https://github.com/fluxcd/flagger/pull/1216)
- include Contour retryOn in the sample canary
  [#1223](https://github.com/fluxcd/flagger/pull/1223)

#### Fixes:
- fix contour prom query for when service name is overwritten
  [#1204](https://github.com/fluxcd/flagger/pull/1204)
- fix contour httproxy annotations overwrite
  [#1205](https://github.com/fluxcd/flagger/pull/1205)
- Fix primary HPA label reconciliation
  [#1215](https://github.com/fluxcd/flagger/pull/1215)
- fix: add finalizers to canaries
  [#1219](https://github.com/fluxcd/flagger/pull/1219)
- typo: boostrap -> bootstrap
  [#1220](https://github.com/fluxcd/flagger/pull/1220)
- typo: controller
  [#1221](https://github.com/fluxcd/flagger/pull/1221)
- update guide for flagger on aliyun ASM
  [#1222](https://github.com/fluxcd/flagger/pull/1222)
- Reintroducing empty check for metric template references.
  [#1224](https://github.com/fluxcd/flagger/pull/1224)


## 1.21.0

**Release date:** 2022-05-06

This release comes with an option to disable cross-namespace references to Kubernetes
custom resources such as `AlertProivders` and `MetricProviders`. When running Flagger
on multi-tenant environments it is advised to set the `-no-cross-namespace-refs=true` flag.

In addition, this version enables Flagger to target Istio and Kuma multi-cluster setups.
When installing Flagger with Helm, the service mesh control plane kubeconfig secret
can be specified using `--set controlplane.kubeconfig.secretName`.

#### Improvements

- Add flag to disable cross namespace refs to custom resources
  [#1181](https://github.com/fluxcd/flagger/pull/1181)
- Rename kubeconfig section in helm values
  [#1188](https://github.com/fluxcd/flagger/pull/1188)
- Update Flagger overview diagram
  [#1187](https://github.com/fluxcd/flagger/pull/1187)

#### Fixes

- Avoid setting owner refs if the service mesh/ingress is on a different cluster
  [#1183](https://github.com/fluxcd/flagger/pull/1183)

## 1.20.0

**Release date:** 2022-04-15

This release comes with improvements to the AppMesh, Contour and Istio integrations.

#### Improvements

- AppMesh: Add annotation to enable Envoy access logs
  [#1156](https://github.com/fluxcd/flagger/pull/1156)
- Contour: Update the httproxy API and enable RetryOn
  [#1164](https://github.com/fluxcd/flagger/pull/1164)
- Istio: Add destination port when port discovery and delegation are true
  [#1145](https://github.com/fluxcd/flagger/pull/1145)
- Metrics: Add canary analysis result as Prometheus metrics
  [#1148](https://github.com/fluxcd/flagger/pull/1148)

#### Fixes

- Fix canary rollback behaviour
  [#1171](https://github.com/fluxcd/flagger/pull/1171)
- Shorten the metric analysis cycle after confirm promotion gate is open
  [#1139](https://github.com/fluxcd/flagger/pull/1139)
- Fix unit of time in the Istio Grafana dashboard
  [#1162](https://github.com/fluxcd/flagger/pull/1162)
- Fix the service toggle condition in the podinfo helm chart
  [#1146](https://github.com/fluxcd/flagger/pull/1146)

## 1.19.0

**Release date:** 2022-03-14

This release comes with support for Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/) v1alpha2.
For more details see the [Gateway API Progressive Delivery tutorial](https://docs.flagger.app/tutorials/gatewayapi-progressive-delivery).

#### Features

- Add Gateway API as a provider
  [#1108](https://github.com/fluxcd/flagger/pull/1108)

#### Improvements

- Add arm64 support for loadtester
  [#1128](https://github.com/fluxcd/flagger/pull/1128)
- Restrict source namespaces in flagger-loadtester
  [#1119](https://github.com/fluxcd/flagger/pull/1119)
- Remove support for Helm v2 in loadtester
  [#1130](https://github.com/fluxcd/flagger/pull/1130)

#### Fixes

- Fix potential canary finalizer duplication
  [#1125](https://github.com/fluxcd/flagger/pull/1125)
- Use the primary replicas when scaling up the canary (no hpa)
  [#1110](https://github.com/fluxcd/flagger/pull/1110)

## 1.18.0

**Release date:** 2022-02-14

This release comes with a new API field called `canaryReadyThreshold`
that allows setting the percentage of pods that need to be available
to consider the canary deployment as ready.

Starting with version, the canary deployment labels, annotations and
replicas fields are copied to the primary deployment at promotion time.

#### Features

- Add field `spec.analysis.canaryReadyThreshold` for configuring canary threshold
  [#1102](https://github.com/fluxcd/flagger/pull/1102)

#### Improvements

- Update metadata during subsequent promote
  [#1092](https://github.com/fluxcd/flagger/pull/1092)
- Set primary deployment `replicas` when autoscaler isn't used
  [#1106](https://github.com/fluxcd/flagger/pull/1106)
- Update `matchLabels` for `TopologySpreadContstraints` in Deployments
  [#1041](https://github.com/fluxcd/flagger/pull/1041)

#### Fixes

- Send warning and error alerts correctly
  [#1105](https://github.com/fluxcd/flagger/pull/1105)
- Fix for when Prometheus returns NaN
  [#1095](https://github.com/fluxcd/flagger/pull/1095)
- docs: Fix typo ExternalDNS
  [#1103](https://github.com/fluxcd/flagger/pull/1103)

## 1.17.0

**Release date:** 2022-01-11

This release comes with support for [Kuma Service Mesh](https://kuma.io/).
For more details see the [Kuma Progressive Delivery tutorial](https://docs.flagger.app/tutorials/kuma-progressive-delivery).

To differentiate alerts based on the cluster name, you can configure Flagger with the `-cluster-name=my-cluster`
command flag, or with Helm `--set clusterName=my-cluster`.

#### Features

- Add kuma support for progressive traffic shifting canaries
  [#1085](https://github.com/fluxcd/flagger/pull/1085)
  [#1093](https://github.com/fluxcd/flagger/pull/1093)

#### Improvements

- Publish a Software Bill of Materials (SBOM)
  [#1094](https://github.com/fluxcd/flagger/pull/1094)
- Add cluster name to flagger cmd args for altering
  [#1041](https://github.com/fluxcd/flagger/pull/1041)

## 1.16.1

**Release date:** 2021-12-17

This release contains updates to Kubernetes packages (1.23.0), Alpine (3.15)
and load tester components.

#### Improvements

- Release loadtester v0.21.0
  [#1083](https://github.com/fluxcd/flagger/pull/1083)
- Add loadtester image pull secrets to Helm chart
  [#1076](https://github.com/fluxcd/flagger/pull/1076)
- Update libraries included in the load tester to newer versions
  [#1063](https://github.com/fluxcd/flagger/pull/1063)
  [#1080](https://github.com/fluxcd/flagger/pull/1080)
- Update Kubernetes packages to v1.23.0
  [#1078](https://github.com/fluxcd/flagger/pull/1078)
- Update Alpine to 3.15
  [#1081](https://github.com/fluxcd/flagger/pull/1081)
- Update Go to v1.17
  [#1077](https://github.com/fluxcd/flagger/pull/1077)

## 1.16.0

**Release date:** 2021-11-22

This release comes with a new API field called `primaryReadyThreshold`
that allows setting the percentage of pods that need to be available
to consider the primary deployment as ready.

#### Features

- Allow configuring threshold for primary
  [#1048](https://github.com/fluxcd/flagger/pull/1048)

#### Improvements

- Append to list of ownerReferences for primary configmaps and secrets
  [#1052](https://github.com/fluxcd/flagger/pull/1052)
- Prevent Flux from overriding Flagger managed objects
  [#1049](https://github.com/fluxcd/flagger/pull/1049)
- Add warning in docs about ExternalDNS + Istio configuration
  [#1044](https://github.com/fluxcd/flagger/pull/1044)

#### Fixes

- Mark `CanaryMetric.Threshold` as omitempty
  [#1047](https://github.com/fluxcd/flagger/pull/1047)
- Replace `ioutil` in testing of gchat
  [#1045](https://github.com/fluxcd/flagger/pull/1045)

## 1.15.0

**Release date:** 2021-10-28

This release comes with support for NGINX ingress canary metrics.
The nginx-ingress minimum supported version is now v1.0.2.

Starting with version, Flagger will use the `spec.service.apex.annotations`
to annotate the generated apex VirtualService, TrafficSplit or HTTPProxy.

#### Features

- Use nginx controller canary metrics
  [#1023](https://github.com/fluxcd/flagger/pull/1023)
- Add metadata annotations to generated apex objects
  [#1034](https://github.com/fluxcd/flagger/pull/1034)

#### Improvements

- Update load tester binaries (CVEs fix)
  [#1038](https://github.com/fluxcd/flagger/pull/1038)
- Add podLabels to load tester Helm chart
  [#1036](https://github.com/fluxcd/flagger/pull/1036)

## 1.14.0

**Release date:** 2021-09-20

This release comes with support for extending the canary analysis with
Dynatrace, InfluxDB and Google Cloud Monitoring (Stackdriver) metrics.

#### Features

- Add Stackdriver metric provider
  [#991](https://github.com/fluxcd/flagger/pull/991)
- Add Influxdb metric provider
  [#1012](https://github.com/fluxcd/flagger/pull/1012)
- Add Dynatrace metric provider
  [#1013](https://github.com/fluxcd/flagger/pull/1013)

#### Fixes

- Fix inline promql query
  [#1015](https://github.com/fluxcd/flagger/pull/1015)
- Fix Istio load balancer settings mapping
  [#1016](https://github.com/fluxcd/flagger/pull/1016)

## 1.13.0

**Release date:** 2021-08-25

This release comes with support for [Open Service Mesh](https://openservicemesh.io).
For more details see the [OSM Progressive Delivery tutorial](https://docs.flagger.app/tutorials/osm-progressive-delivery).

Starting with this version, Flagger container images are signed with
[sigstore/cosign](https://github.com/sigstore/cosign), for more details see the
[Flagger cosign docs](https://github.com/fluxcd/flagger/blob/main/.cosign/README.md).

#### Features

- Support OSM progressive traffic shifting in Flagger
  [#955](https://github.com/fluxcd/flagger/pull/955)
  [#977](https://github.com/fluxcd/flagger/pull/977)
- Add support for Google Chat alerts
  [#953](https://github.com/fluxcd/flagger/pull/953)

#### Improvements

- Sign Flagger container images with cosign
  [#983](https://github.com/fluxcd/flagger/pull/983)
- Update Gloo APIs and e2e tests to Gloo v1.8.9
  [#982](https://github.com/fluxcd/flagger/pull/982)
- Update e2e tests to Istio v1.11, Contour v1.18, Linkerd v2.10.2 and NGINX v0.49.0
  [#979](https://github.com/fluxcd/flagger/pull/979)
- Update e2e tests to Traefik to 2.4.9
  [#960](https://github.com/fluxcd/flagger/pull/960)
- Add support for volumes/volumeMounts in loadtester Helm chart
  [#975](https://github.com/fluxcd/flagger/pull/975)
- Add extra podLabels options to Flagger Helm Chart
  [#966](https://github.com/fluxcd/flagger/pull/966)

#### Fixes

- Fix for the http client proxy overriding the default client
  [#943](https://github.com/fluxcd/flagger/pull/943)
- Drop deprecated io/ioutil
  [#964](https://github.com/fluxcd/flagger/pull/964)
- Remove problematic nulls from Grafana dashboard
  [#952](https://github.com/fluxcd/flagger/pull/952)

## 1.12.1

**Release date:** 2021-06-17

This release comes with a fix to Flagger when used with Flux v2.

#### Improvements

- Update Go to v1.16 and Kubernetes packages to v1.21.1
  [#940](https://github.com/fluxcd/flagger/pull/940)

#### Fixes

- Remove the GitOps Toolkit metadata from generated objects
  [#939](https://github.com/fluxcd/flagger/pull/939)

## 1.12.0

**Release date:** 2021-06-16

This release comes with support for disabling the SSL certificate verification
for the Prometheus and Graphite metric providers.

#### Improvements
 
- Add `insecureSkipVerify` option for Prometheus and Graphite
  [#935](https://github.com/fluxcd/flagger/pull/935)
- Copy labels from Gloo upstreams
  [#932](https://github.com/fluxcd/flagger/pull/932)
- Improve language and correct typos in FAQs docs
  [#925](https://github.com/fluxcd/flagger/pull/925)
- Remove Flux GC markers from generated objects
  [#936](https://github.com/fluxcd/flagger/pull/936)

#### Fixes

- Require SMI TrafficSplit Service and Weight
  [#878](https://github.com/fluxcd/flagger/pull/878)
 
## 1.11.0

**Release date:** 2021-06-01

**Breaking change:** the minimum supported version of Kubernetes is v1.19.0.

This release comes with support for Kubernetes Ingress `networking.k8s.io/v1`.
The Ingress from `networking.k8s.io/v1beta1` is no longer supported,
affected integrations: **NGINX** and **Skipper** ingress controllers.

#### Improvements

- Upgrade Ingress to networking.k8s.io/v1
  [#917](https://github.com/fluxcd/flagger/pull/917)
- Update Kubernetes manifests to rbac.authorization.k8s.io/v1
  [#920](https://github.com/fluxcd/flagger/pull/920)

## 1.10.0

**Release date:** 2021-05-28

This release comes with support for [Graphite](https://docs.flagger.app/usage/metrics#graphite) metric templates.

#### Features

- Add Graphite metrics provider
  [#915](https://github.com/fluxcd/flagger/pull/915)

#### Improvements

- ConfigTracker: Scan envFrom in init-containers
  [#914](https://github.com/fluxcd/flagger/pull/914)
- e2e: Update Istio to v1.10 and Contour to v1.15
  [#914](https://github.com/fluxcd/flagger/pull/914)

## 1.9.0

**Release date:** 2021-05-14

This release comes with improvements to the [Gloo Edge](https://docs.flagger.app/tutorials/gloo-progressive-delivery) integration.

Starting with this version, Flagger no longer requires Gloo discovery to be enabled.
Flagger generated the Gloo upstream objects on its own and optionally it can use an
existing upstream (specified with `.spec.upstreamRef`) as a template. 

#### Features

- Gloo: Create gloo upstreams from non-discovered services
  [#894](https://github.com/fluxcd/flagger/pull/894)
- Gloo Upstream Ref for Upstream Config
  [#908](https://github.com/fluxcd/flagger/pull/908)

#### Improvements

- Adjusted Nginx ingress canary headers on init and promotion
  [#907](https://github.com/fluxcd/flagger/pull/907)

## 1.8.0

**Release date:** 2021-04-29

This release comes with support for the SMI `v1alpha2` and `v1alpha3` TrafficSplit APIs.

For SMI compatible service mesh solutions like Open Service Mesh, Consul Connect or Nginx Service Mesh,
[Prometheus MetricTemplates](https://docs.flagger.app/usage/metrics#prometheus) can be used to implement
the request success rate and request duration checks.

The desired SMI version can be set in the Canary object:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: my-canary
spec:
  provider: "smi:v1alpha3" # or "smi:v1alpha2"
```

#### Features

- Implement SMI v1alpha2 and v1alpha3 routers
  [#896](https://github.com/fluxcd/flagger/pull/896)
  [#879](https://github.com/fluxcd/flagger/pull/879)
- Add alerting HTTP/S proxy option
  [#872](https://github.com/fluxcd/flagger/pull/872)
- Add option to mute alerts generated from webhooks
  [#887](https://github.com/fluxcd/flagger/pull/887)

#### Fixes

- Scale up canary on confirm rollout
  [#878](https://github.com/fluxcd/flagger/pull/878)

## 1.7.0

**Release date:** 2021-03-23

This release comes with support for manually approving
the traffic weight increase.

#### Features

- Add webhook for manually approving traffic weight increase
  [#849](https://github.com/fluxcd/flagger/pull/849)
- Add WaitingPromotion phase to canary status
  [#859](https://github.com/fluxcd/flagger/pull/859)

#### Improvements

- linkerd: update prometheus URL based on the latest 2.10 changes
  [#845](https://github.com/fluxcd/flagger/pull/845)
- docs: update resources to disable mTLS in Istio
  [#843](https://github.com/fluxcd/flagger/pull/843)
- docs: updating slack alerting docs to point to legacy slack webhooks
  [#833](https://github.com/fluxcd/flagger/pull/833)
- chart: Add pull secret for Prometheus deployment
  [#842](https://github.com/fluxcd/flagger/pull/842)
- Update Kubernetes packages to v1.20.4
  [#857](https://github.com/fluxcd/flagger/pull/857)

## 1.6.4

**Release date:** 2021-02-26

This release comes with a bug fix to the AppMesh integration
when using multiple backends.

#### Improvements

- Consolidate logos and add project name logos
  [#829](https://github.com/fluxcd/flagger/pull/829)
- chart: add env option to loadtester
  [#821](https://github.com/fluxcd/flagger/pull/821)
- chart: Added PodDisruptionBudget for the loadtester
  [#819](https://github.com/fluxcd/flagger/pull/819)

#### Fixes

- Fix AWS AppMesh issue when providing multiple backends
  [#831](https://github.com/fluxcd/flagger/pull/831)

## 1.6.3

**Release date:** 2021-02-15

This release comes with support for Kubernetes pod topology spread constraints.

Flagger has a new [logo](https://github.com/fluxcd/flagger/pull/812),
many thanks to [Bianca](https://github.com/bia) for designed it.

#### Improvements

- Rewrite the primary Pod Topology Spread Constraints based on label selector
  [#806](https://github.com/fluxcd/flagger/pull/806)

#### Fixes

- Suffix only the podAntiAffinity values that match the deployment name
  [#805](https://github.com/fluxcd/flagger/pull/805)
- Check if mandatory secrets/configmaps exist
  [#799](https://github.com/fluxcd/flagger/pull/799)

## 1.6.2

**Release date:** 2021-01-28

This release comes with support for Kubernetes anti-affinity rules.

#### Improvements

- Support for adding `-primary` suffix to Anti-Affinity values
  [#788](https://github.com/fluxcd/flagger/pull/788)

#### Fixes

- Add missing alerts section to Canary CRD schema
  [#794](https://github.com/fluxcd/flagger/pull/794)
  
## 1.6.1

**Release date:** 2021-01-19

This release extends the support for Istio's `HTTPMatchRequest` and
comes with a regression bug fix to secrets and configmaps tracking.

#### Improvements

- Update HTTPMatchRequest to match Istio's definitions
  [#777](https://github.com/fluxcd/flagger/pull/777)
- e2e: Update Istio to v1.8.2 and Contour to v1.11.0
  [#778](https://github.com/fluxcd/flagger/pull/778)

#### Fixes

- Add missing TrackedConfig field to Canary status CRD
  [#781](https://github.com/fluxcd/flagger/pull/781)
  
## 1.6.0

**Release date:** 2021-01-05

**Breaking change:** the minimum supported version of Kubernetes is v1.16.0.

This release comes with support for A/B testing using [Gloo Edge](https://docs.flagger.app/tutorials/gloo-progressive-delivery) 
HTTP headers based routing.

#### Features

- A/B testing support for Gloo Edge ingress controller
  [#765](https://github.com/fluxcd/flagger/pull/765)

#### Improvements

- Upgrade the Kubernetes packages to `v1.20.1` and Flagger's CRDs to `apiextensions.k8s.io/v1` 
  [#772](https://github.com/fluxcd/flagger/pull/772)
  
## 1.5.0

**Release date:** 2020-12-22

This is the first release of Flagger under [fluxcd](https://github.com/fluxcd) organization (CNCF sandbox).
Starting with this version, Flagger can be installed on multi-arch Kubernetes clusters (Linux AMD64/ARM64/ARM).
The multi-arch image is available on GitHub Container Registry
at [ghcr.io/fluxcd/flagger](https://github.com/orgs/fluxcd/packages/container/package/flagger).

#### Improvements

- Publish multi-arch image to GitHub Container Registry
  [#763](https://github.com/fluxcd/flagger/pull/763)
- Migrate CI to GitHub Actions
  [#754](https://github.com/fluxcd/flagger/pull/754)
- Add e2e tests for label prefix inclusion
  [#762](https://github.com/fluxcd/flagger/pull/762)  
- Added PodDisruptionBudget to the Flagger Helm chart
  [#749](https://github.com/fluxcd/flagger/pull/749)

## v1.4.2

**Release date:** 2020-12-09

Fix Istio virtual service delegation

#### Improvements

- Add Prometheus basic-auth config to docs
    [#746](https://github.com/fluxcd/flagger/pull/746)
- Update Prometheus to 2.23.0 and Grafana to 7.3.4
    [#747](https://github.com/fluxcd/flagger/pull/747)

#### Fixes

- Fix for VirtualService delegation when analysis is enabled
    [#745](https://github.com/fluxcd/flagger/pull/745)

## 1.4.1 (2020-12-08) 

Prevent primary ConfigMaps and Secrets from being pruned by Flux

#### Improvements

- Apply label prefix rules for ConfigMaps and Secrets
    [#743](https://github.com/fluxcd/flagger/pull/743)

## 1.4.0 (2020-12-07) 

Add support for Traefik ingress controller

#### Features

- Add Traefik support for progressive traffic shifting with `TraefikService`
    [#736](https://github.com/fluxcd/flagger/pull/736)
- Add support for HPA v2beta2 behaviors
    [#740](https://github.com/fluxcd/flagger/pull/740)

## 1.3.0 (2020-11-23) 

Add support for custom weights when configuring traffic shifting

#### Features

- Support AWS App Mesh backends ARN
    [#715](https://github.com/fluxcd/flagger/pull/715)
- Add support for Istio VirtualService delegation
    [#715](https://github.com/fluxcd/flagger/pull/715)
- Copy labels from canary to primary workloads based on prefix rules
    [#709](https://github.com/fluxcd/flagger/pull/709)

#### Improvements

- Add QPS and Burst configs for kubernetes client
    [#725](https://github.com/fluxcd/flagger/pull/725)
- Update Istio to v1.8.0
    [#733](https://github.com/fluxcd/flagger/pull/733)

## 1.2.0 (2020-09-29) 

Add support for New Relic metrics

#### Features

- Add New Relic as a metrics provider
    [#691](https://github.com/fluxcd/flagger/pull/691)

#### Improvements

- Derive the label selector value from the target matchLabel
    [#685](https://github.com/fluxcd/flagger/pull/685)
- Preserve Skipper predicates
    [#681](https://github.com/fluxcd/flagger/pull/681)

#### Fixes

- Do not promote when not ready on skip analysis
    [#695](https://github.com/fluxcd/flagger/pull/695)
    
## 1.1.0 (2020-08-18) 

Add support for Skipper ingress controller

#### Features

- Skipper Ingress Controller support
    [#670](https://github.com/fluxcd/flagger/pull/670)
- Support per-config configTracker disable via ConfigMap/Secret annotation
    [#671](https://github.com/fluxcd/flagger/pull/671)

#### Improvements

- Add priorityClassName and securityContext to Helm charts
    [#652](https://github.com/fluxcd/flagger/pull/652)
    [#668](https://github.com/fluxcd/flagger/pull/668)
- Update Kubernetes packages to v1.18.8
    [#672](https://github.com/fluxcd/flagger/pull/672)
- Update Istio, Linkerd and Contour e2e tests
    [#661](https://github.com/fluxcd/flagger/pull/661)

#### Fixes

- Fix O(log n) bug over network in GetTargetConfigs
    [#663](https://github.com/fluxcd/flagger/pull/663)
- Fix(grafana): metrics change since Kubernetes 1.16
    [#663](https://github.com/fluxcd/flagger/pull/663)

## 1.0.1 (2020-07-18) 

Add support for App Mesh Gateway GA

#### Improvements

- Update App Mesh docs to v1beta2 API
    [#649](https://github.com/fluxcd/flagger/pull/649)
- Add threadiness to Flagger helm chart
    [#643](https://github.com/fluxcd/flagger/pull/643)
- Add Istio virtual service to loadtester helm chart
    [#643](https://github.com/fluxcd/flagger/pull/643)

#### Fixes

- Fix multiple paths per rule on canary ingress
    [#632](https://github.com/fluxcd/flagger/pull/632)
- Fix installers for kustomize >= 3.6.0
    [#646](https://github.com/fluxcd/flagger/pull/646)

## 1.0.0 (2020-06-17) 

This is the GA release for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

Two new resources were added to the API: `MetricTemplate` and `AlertProvider`.
The analysis can reference [metric templates](https://docs.flagger.app//usage/metrics#custom-metrics)
to query Prometheus, Datadog and AWS CloudWatch.
[Alerting](https://docs.flagger.app/v/master/usage/alerting#canary-configuration) can be configured on a per
canary basis for Slack, MS Teams, Discord and Rocket.

#### Features

- Implement progressive promotion
    [#593](https://github.com/fluxcd/flagger/pull/593)

#### Improvements

- istio: Add source labels to analysis matching rules
    [#594](https://github.com/fluxcd/flagger/pull/594)
- istio: Add allow origins field to CORS spec
    [#604](https://github.com/fluxcd/flagger/pull/604)
- istio: Change builtin metrics to work with Istio telemetry v2
    [#623](https://github.com/fluxcd/flagger/pull/623)
- appmesh: Implement App Mesh v1beta2 timeout
    [#611](https://github.com/fluxcd/flagger/pull/611)
- metrics: Check metrics server availability during canary initialization
    [#592](https://github.com/fluxcd/flagger/pull/592)

## 1.0.0-rc.5 (2020-05-14) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

#### Features

- Add support for AWS AppMesh v1beta2 API
    [#584](https://github.com/fluxcd/flagger/pull/584)
- Add support for Contour v1.4 ingress class
    [#588](https://github.com/fluxcd/flagger/pull/588)
- Add user-specified labels/annotations to the generated Services
    [#538](https://github.com/fluxcd/flagger/pull/538)

#### Improvements

- Support compatible Prometheus service
    [#557](https://github.com/fluxcd/flagger/pull/557)
- Update e2e tests and packages to Kubernetes v1.18
    [#549](https://github.com/fluxcd/flagger/pull/549)
    [#576](https://github.com/fluxcd/flagger/pull/576)

#### Fixes

- pkg/controller: retry canary initialization on conflict
    [#586](https://github.com/fluxcd/flagger/pull/586)

## 1.0.0-rc.4 (2020-04-03) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

**Breaking change**: the minimum supported version of Kubernetes is v1.14.0.

#### Features

- Implement NGINX Ingress header regex matching
    [#546](https://github.com/fluxcd/flagger/pull/546)

#### Improvements

- pkg/router: update ingress API to networking.k8s.io/v1beta1
    [#534](https://github.com/fluxcd/flagger/pull/534)
- loadtester: add return cmd output option
    [#535](https://github.com/fluxcd/flagger/pull/535)
- refactoring: finalizer error handling and unit testing
    [#531](https://github.com/fluxcd/flagger/pull/535)
    [#530](https://github.com/fluxcd/flagger/pull/530)
- chart: add finalizers to RBAC rules for OpenShift
    [#537](https://github.com/fluxcd/flagger/pull/537)
- chart: allow security context to be disabled on OpenShift
    [#543](https://github.com/fluxcd/flagger/pull/543)
- chart: add annotations for service account 
    [#521](https://github.com/fluxcd/flagger/pull/521)
- docs: Add Prometheus Operator tutorial
    [#524](https://github.com/fluxcd/flagger/pull/524)

#### Fixes

- pkg/controller: avoid status conflicts on initialization
    [#544](https://github.com/fluxcd/flagger/pull/544)
- pkg/canary: fix status retry
    [#541](https://github.com/fluxcd/flagger/pull/541)
- loadtester: fix timeout errors
    [#539](https://github.com/fluxcd/flagger/pull/539)
- pkg/canary/daemonset: fix readiness check
    [#529](https://github.com/fluxcd/flagger/pull/529)
- logs: reduce log verbosity and fix typos
    [#540](https://github.com/fluxcd/flagger/pull/540)
    [#526](https://github.com/fluxcd/flagger/pull/526)


## 1.0.0-rc.3 (2020-03-23) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

#### Features

- Add opt-in finalizers to revert Flagger's mutations on deletion of a canary
    [#495](https://github.com/fluxcd/flagger/pull/495)

#### Improvements

- e2e: update end-to-end tests to Contour 1.3.0 and Gloo 1.3.14
    [#519](https://github.com/fluxcd/flagger/pull/519)
- build: update Kubernetes packages to 1.17.4
    [#516](https://github.com/fluxcd/flagger/pull/516)

#### Fixes

- Preserve node ports on service reconciliation
    [#514](https://github.com/fluxcd/flagger/pull/514)

## 1.0.0-rc.2 (2020-03-19) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

#### Features

- Make mirror percentage configurable when using Istio traffic shadowing
    [#492](https://github.com/fluxcd/flagger/pull/455)
- Add support for running Concord tests with loadtester webhooks
    [#507](https://github.com/fluxcd/flagger/pull/507)

#### Improvements

- docs: add Istio telemetry v2 upgrade guide
    [#486](https://github.com/fluxcd/flagger/pull/486),
    update A/B testing tutorial for Istio 1.5
    [#502](https://github.com/fluxcd/flagger/pull/502),
    add how to retry a failed release to FAQ
    [#494](https://github.com/fluxcd/flagger/pull/494)
- e2e: update end-to-end tests to
    Istio 1.5 [#447](https://github.com/fluxcd/flagger/pull/447) and
    NGINX Ingress 0.30
    [#489](https://github.com/fluxcd/flagger/pull/489)
    [#511](https://github.com/fluxcd/flagger/pull/511)
- refactoring:
    error handling [#480](https://github.com/fluxcd/flagger/pull/480),
    scheduler [#484](https://github.com/fluxcd/flagger/pull/484) and 
    unit tests [#475](https://github.com/fluxcd/flagger/pull/475)
- chart: add the log level configuration to Flagger helm chart
    [#506](https://github.com/fluxcd/flagger/pull/506)

#### Fixes

- Fix nil pointer for the global notifiers [#504](https://github.com/fluxcd/flagger/pull/504)

## 1.0.0-rc.1 (2020-03-03) 

This is a release candidate for Flagger v1.0.0.

The upgrade procedure from 0.x to 1.0 can be found [here](https://docs.flagger.app/dev/upgrade-guide).

Two new resources were added to the API: `MetricTemplate` and `AlertProvider`.
The analysis can reference [metric templates](https://docs.flagger.app//usage/metrics#custom-metrics)
to query Prometheus, Datadog and AWS CloudWatch.
[Alerting](https://docs.flagger.app/v/master/usage/alerting#canary-configuration) can be configured on a per
canary basis for Slack, MS Teams, Discord and Rocket.

#### Features

- Implement metric templates for Prometheus [#419](https://github.com/fluxcd/flagger/pull/419),
    Datadog [#460](https://github.com/fluxcd/flagger/pull/460) and
    CloudWatch [#464](https://github.com/fluxcd/flagger/pull/464)
- Implement metric range validation [#424](https://github.com/fluxcd/flagger/pull/424)
- Add support for targeting DaemonSets [#455](https://github.com/fluxcd/flagger/pull/455)
- Implement canary alerts and alert providers (Slack, MS Teams, Discord and Rocket)
    [#429](https://github.com/fluxcd/flagger/pull/429)

#### Improvements

- Add support for Istio multi-cluster
    [#447](https://github.com/fluxcd/flagger/pull/447) [#450](https://github.com/fluxcd/flagger/pull/450)
- Extend Istio traffic policy [#441](https://github.com/fluxcd/flagger/pull/441),
    add support for header operations [#442](https://github.com/fluxcd/flagger/pull/442) and 
    set ingress destination port when multiple ports are discovered [#436](https://github.com/fluxcd/flagger/pull/436)
- Add support for rollback gating [#449](https://github.com/fluxcd/flagger/pull/449)
- Allow disabling ConfigMaps and Secrets tracking [#425](https://github.com/fluxcd/flagger/pull/425)

#### Fixes

- Fix spec changes detection [#446](https://github.com/fluxcd/flagger/pull/446)
- Track projected ConfigMaps and Secrets [#433](https://github.com/fluxcd/flagger/pull/433)

## 0.23.0 (2020-02-06) 

Adds support for service name configuration and rollback webhook

#### Features

- Implement service name override [#416](https://github.com/fluxcd/flagger/pull/416)
- Add support for gated rollback [#420](https://github.com/fluxcd/flagger/pull/420)

## 0.22.0 (2020-01-16) 

Adds event dispatching through webhooks

#### Features

- Implement event dispatching webhook [#409](https://github.com/fluxcd/flagger/pull/409)
- Add general purpose event webhook [#401](https://github.com/fluxcd/flagger/pull/401)

#### Improvements

- Update Contour to v1.1 and add Linkerd header [#411](https://github.com/fluxcd/flagger/pull/411)
- Update Istio e2e to v1.4.3 [#407](https://github.com/fluxcd/flagger/pull/407)
- Update Kubernetes packages to 1.17 [#406](https://github.com/fluxcd/flagger/pull/406)

## 0.21.0 (2020-01-06) 

Adds support for Contour ingress controller

#### Features

- Add support for Contour ingress controller [#397](https://github.com/fluxcd/flagger/pull/397)
- Add support for Envoy managed by Crossover via SMI [#386](https://github.com/fluxcd/flagger/pull/386)
- Extend canary target ref to Kubernetes Service kind [#372](https://github.com/fluxcd/flagger/pull/372)

#### Improvements 

- Add Prometheus operator PodMonitor template to Helm chart [#399](https://github.com/fluxcd/flagger/pull/399)
- Update e2e tests to Kubernetes v1.16 [#390](https://github.com/fluxcd/flagger/pull/390)

## 0.20.4 (2019-12-03) 

Adds support for taking over a running deployment without disruption

#### Improvements 

- Add initialization phase to Kubernetes router [#384](https://github.com/fluxcd/flagger/pull/384)
- Add canary controller interface and Kubernetes deployment kind implementation [#378](https://github.com/fluxcd/flagger/pull/378)

#### Fixes

- Skip primary check on skip analysis [#380](https://github.com/fluxcd/flagger/pull/380)

## 0.20.3 (2019-11-13) 

Adds wrk to load tester tools and the App Mesh gateway chart to Flagger Helm repository

#### Improvements 

- Add wrk to load tester tools [#368](https://github.com/fluxcd/flagger/pull/368)
- Add App Mesh gateway chart [#365](https://github.com/fluxcd/flagger/pull/365)

## 0.20.2 (2019-11-07) 

Adds support for exposing canaries outside the cluster using App Mesh Gateway annotations 

#### Improvements 

- Expose canaries on public domains with App Mesh Gateway [#358](https://github.com/fluxcd/flagger/pull/358)

#### Fixes

- Use the specified replicas when scaling up the canary [#363](https://github.com/fluxcd/flagger/pull/363)

## 0.20.1 (2019-11-03) 

Fixes promql execution and updates the load testing tools

#### Improvements 

- Update load tester Helm tools [#8349dd1](https://github.com/fluxcd/flagger/commit/8349dd1cda59a741c7bed9a0f67c0fc0fbff4635)
- e2e testing: update providers [#346](https://github.com/fluxcd/flagger/pull/346)

#### Fixes

- Fix Prometheus query escape [#353](https://github.com/fluxcd/flagger/pull/353)
- Updating hey release link [#350](https://github.com/fluxcd/flagger/pull/350)

## 0.20.0 (2019-10-21) 

Adds support for [A/B Testing](https://docs.flagger.app/usage/progressive-delivery#traffic-mirroring)
and retry policies when using App Mesh

#### Features

- Implement App Mesh A/B testing based on HTTP headers match conditions [#340](https://github.com/fluxcd/flagger/pull/340)
- Implement App Mesh HTTP retry policy [#338](https://github.com/fluxcd/flagger/pull/338)
- Implement metrics server override [#342](https://github.com/fluxcd/flagger/pull/342)

#### Improvements 

- Add the app/name label to services and primary deployment [#333](https://github.com/fluxcd/flagger/pull/333)
- Allow setting Slack and Teams URLs with env vars [#334](https://github.com/fluxcd/flagger/pull/334)
- Refactor Gloo integration [#344](https://github.com/fluxcd/flagger/pull/344)

#### Fixes

- Generate unique names for App Mesh virtual routers and routes [#336](https://github.com/fluxcd/flagger/pull/336)

## 0.19.0 (2019-10-08) 

Adds support for canary and blue/green [traffic mirroring](https://docs.flagger.app/usage/progressive-delivery#traffic-mirroring)

#### Features

- Add traffic mirroring for Istio service mesh [#311](https://github.com/fluxcd/flagger/pull/311)
- Implement canary service target port [#327](https://github.com/fluxcd/flagger/pull/327)

#### Improvements 

- Allow gRPC protocol for App Mesh [#325](https://github.com/fluxcd/flagger/pull/325)
- Enforce blue/green when using Kubernetes networking [#326](https://github.com/fluxcd/flagger/pull/326)

#### Fixes

- Fix port discovery diff [#324](https://github.com/fluxcd/flagger/pull/324)
- Helm chart: Enable Prometheus scraping of Flagger metrics
    [#2141d88](https://github.com/fluxcd/flagger/commit/2141d88ce1cc6be220dab34171c215a334ecde24)

## 0.18.6 (2019-10-03) 

Adds support for App Mesh conformance tests and latency metric checks

#### Improvements 

- Add support for acceptance testing when using App Mesh [#322](https://github.com/fluxcd/flagger/pull/322)
- Add Kustomize installer for App Mesh [#310](https://github.com/fluxcd/flagger/pull/310)
- Update Linkerd to v2.5.0 and Prometheus to v2.12.0 [#323](https://github.com/fluxcd/flagger/pull/323)

#### Fixes

- Fix slack/teams notification fields mapping [#318](https://github.com/fluxcd/flagger/pull/318)

## 0.18.5 (2019-10-02) 

Adds support for [confirm-promotion](https://docs.flagger.app/how-it-works#webhooks)
webhooks and blue/green deployments when using a service mesh

#### Features

- Implement confirm-promotion hook [#307](https://github.com/fluxcd/flagger/pull/307)
- Implement B/G for service mesh providers [#305](https://github.com/fluxcd/flagger/pull/305)

#### Improvements 

- Canary promotion improvements to avoid dropping in-flight requests [#310](https://github.com/fluxcd/flagger/pull/310)
- Update end-to-end tests to Kubernetes v1.15.3 and Istio 1.3.0 [#306](https://github.com/fluxcd/flagger/pull/306) 

#### Fixes

- Skip primary check for App Mesh [#315](https://github.com/fluxcd/flagger/pull/315)

## 0.18.4 (2019-09-08) 

Adds support for NGINX custom annotations and Helm v3 acceptance testing

#### Features

- Add annotations prefix for NGINX ingresses [#293](https://github.com/fluxcd/flagger/pull/293)
- Add wide columns in CRD [#289](https://github.com/fluxcd/flagger/pull/289)
- loadtester: implement Helm v3 test command [#296](https://github.com/fluxcd/flagger/pull/296)
- loadtester: add gRPC health check to load tester image [#295](https://github.com/fluxcd/flagger/pull/295)

#### Fixes

- loadtester: fix tests error logging [#286](https://github.com/fluxcd/flagger/pull/286)

## 0.18.3 (2019-08-22) 

Adds support for tillerless helm tests and protobuf health checking

#### Features

- loadtester: add support for tillerless helm [#280](https://github.com/fluxcd/flagger/pull/280)
- loadtester: add support for protobuf health checking [#280](https://github.com/fluxcd/flagger/pull/280)

#### Improvements 

- Set HTTP listeners for AppMesh virtual routers [#272](https://github.com/fluxcd/flagger/pull/272)

#### Fixes

- Add missing fields to CRD validation spec [#271](https://github.com/fluxcd/flagger/pull/271)
- Fix App Mesh backends validation in CRD [#281](https://github.com/fluxcd/flagger/pull/281)

## 0.18.2 (2019-08-05) 

Fixes multi-port support for Istio

#### Fixes

- Fix port discovery for multiple port services [#267](https://github.com/fluxcd/flagger/pull/267)

#### Improvements 

- Update e2e testing to Istio v1.2.3, Gloo v0.18.8 and NGINX ingress chart v1.12.1 [#268](https://github.com/fluxcd/flagger/pull/268)

## 0.18.1 (2019-07-30) 

Fixes Blue/Green style deployments for Kubernetes and Linkerd providers

#### Fixes

- Fix Blue/Green metrics provider and add e2e tests [#261](https://github.com/fluxcd/flagger/pull/261)

## 0.18.0 (2019-07-29) 

Adds support for [manual gating](https://docs.flagger.app/how-it-works#manual-gating) and pausing/resuming an ongoing analysis

#### Features

- Implement confirm rollout gate, hook and API [#251](https://github.com/fluxcd/flagger/pull/251)

#### Improvements 

- Refactor canary change detection and status [#240](https://github.com/fluxcd/flagger/pull/240)
- Implement finalising state [#257](https://github.com/fluxcd/flagger/pull/257)
- Add gRPC load testing tool [#248](https://github.com/fluxcd/flagger/pull/248)

#### Breaking changes

- Due to the status sub-resource changes in [#240](https://github.com/fluxcd/flagger/pull/240),
    when upgrading Flagger the canaries status phase will be reset to `Initialized`
- Upgrading Flagger with Helm will fail due to Helm poor support of CRDs,
    see [workaround](https://github.com/fluxcd/flagger/issues/223)

## 0.17.0 (2019-07-08) 

Adds support for Linkerd (SMI Traffic Split API), MS Teams notifications and HA mode with leader election

#### Features

- Add Linkerd support [#230](https://github.com/fluxcd/flagger/pull/230)
- Implement MS Teams notifications [#235](https://github.com/fluxcd/flagger/pull/235)
- Implement leader election [#236](https://github.com/fluxcd/flagger/pull/236)

#### Improvements 

- Add [Kustomize](https://docs.flagger.app/install/flagger-install-on-kubernetes#install-flagger-with-kustomize)
    installer [#232](https://github.com/fluxcd/flagger/pull/232)
- Add Pod Security Policy to Helm chart [#234](https://github.com/fluxcd/flagger/pull/234)

## 0.16.0 (2019-06-23) 

Adds support for running [Blue/Green deployments](https://docs.flagger.app/usage/blue-green)
without a service mesh or ingress controller

#### Features

- Allow blue/green deployments without a service mesh provider [#211](https://github.com/fluxcd/flagger/pull/211)
- Add the service mesh provider to the canary spec [#217](https://github.com/fluxcd/flagger/pull/217)
- Allow multi-port services and implement port discovery [#207](https://github.com/fluxcd/flagger/pull/207)

#### Improvements 

- Add [FAQ page](https://docs.flagger.app/faq) to docs website
- Switch to go modules in CI [#218](https://github.com/fluxcd/flagger/pull/218)
- Update e2e testing to Kubernetes Kind 0.3.0 and Istio 1.2.0

#### Fixes

- Update the primary HPA on canary promotion [#216](https://github.com/fluxcd/flagger/pull/216)

## 0.15.0 (2019-06-12) 

Adds support for customising the Istio [traffic policy](https://docs.flagger.app/how-it-works#istio-routing) in the canary service spec

#### Features

- Generate Istio destination rules and allow traffic policy customisation [#200](https://github.com/fluxcd/flagger/pull/200)

#### Improvements 

-  Update Kubernetes packages to 1.14 and use go modules instead of dep [#202](https://github.com/fluxcd/flagger/pull/202) 

## 0.14.1 (2019-06-05) 

Adds support for running [acceptance/integration tests](https://docs.flagger.app/how-it-works#integration-testing)
with Helm test or Bash Bats using pre-rollout hooks

#### Features

- Implement Helm and Bash pre-rollout hooks [#196](https://github.com/fluxcd/flagger/pull/196)

#### Fixes

- Fix promoting canary when max weight is not a multiple of step [#190](https://github.com/fluxcd/flagger/pull/190)
- Add ability to set Prometheus url with custom path without trailing '/' [#197](https://github.com/fluxcd/flagger/pull/197)

## 0.14.0 (2019-05-21) 

Adds support for Service Mesh Interface and [Gloo](https://docs.flagger.app/usage/gloo-progressive-delivery) ingress controller

#### Features

- Add support for SMI (Istio weighted traffic) [#180](https://github.com/fluxcd/flagger/pull/180)
- Add support for Gloo ingress controller (weighted traffic) [#179](https://github.com/fluxcd/flagger/pull/179)

## 0.13.2 (2019-04-11) 

Fixes for Jenkins X deployments (prevent the jx GC from removing the primary instance)

#### Fixes

- Do not copy labels from canary to primary deployment [#178](https://github.com/fluxcd/flagger/pull/178)

#### Improvements 

- Add NGINX ingress controller e2e and unit tests [#176](https://github.com/fluxcd/flagger/pull/176) 

## 0.13.1 (2019-04-09) 

Fixes for custom metrics checks and NGINX Prometheus queries 

#### Fixes

- Fix promql queries for custom checks and NGINX [#174](https://github.com/fluxcd/flagger/pull/174)

## 0.13.0 (2019-04-08) 

Adds support for [NGINX](https://docs.flagger.app/usage/nginx-progressive-delivery) ingress controller

#### Features

- Add support for nginx ingress controller (weighted traffic and A/B testing) [#170](https://github.com/fluxcd/flagger/pull/170)
- Add Prometheus add-on to Flagger Helm chart for App Mesh and
    NGINX [79b3370](https://github.com/fluxcd/flagger/pull/170/commits/79b337089294a92961bc8446fd185b38c50a32df)

#### Fixes

- Fix duplicate hosts Istio error when using wildcards [#162](https://github.com/fluxcd/flagger/pull/162)

## 0.12.0 (2019-04-29) 

Adds support for [SuperGloo](https://docs.flagger.app/install/flagger-install-with-supergloo)

#### Features

- Supergloo support for canary deployment (weighted traffic) [#151](https://github.com/fluxcd/flagger/pull/151)

## 0.11.1 (2019-04-18) 

Move Flagger and the load tester container images to Docker Hub

#### Features

- Add Bash Automated Testing System support to Flagger tester for running acceptance tests as pre-rollout hooks 

## 0.11.0 (2019-04-17) 

Adds pre/post rollout [webhooks](https://docs.flagger.app/how-it-works#webhooks)

#### Features

- Add `pre-rollout` and `post-rollout` webhook types [#147](https://github.com/fluxcd/flagger/pull/147)

#### Improvements 

- Unify App Mesh and Istio builtin metric checks [#146](https://github.com/fluxcd/flagger/pull/146) 
- Make the pod selector label configurable [#148](https://github.com/fluxcd/flagger/pull/148)

#### Breaking changes

- Set default `mesh` Istio gateway only if no gateway is specified [#141](https://github.com/fluxcd/flagger/pull/141)

## 0.10.0 (2019-03-27) 

Adds support for App Mesh

#### Features

- AWS App Mesh integration
    [#107](https://github.com/fluxcd/flagger/pull/107)
    [#123](https://github.com/fluxcd/flagger/pull/123)

#### Improvements 

- Reconcile Kubernetes ClusterIP services [#122](https://github.com/fluxcd/flagger/pull/122) 
 
#### Fixes

- Preserve pod labels on canary promotion [#105](https://github.com/fluxcd/flagger/pull/105)
- Fix canary status Prometheus metric [#121](https://github.com/fluxcd/flagger/pull/121)

## 0.9.0 (2019-03-11)

Allows A/B testing scenarios where instead of weighted routing, the traffic is split between the 
primary and canary based on HTTP headers or cookies.

#### Features

- A/B testing - canary with session affinity [#88](https://github.com/fluxcd/flagger/pull/88)

#### Fixes

- Update the analysis interval when the custom resource changes [#91](https://github.com/fluxcd/flagger/pull/91)

## 0.8.0 (2019-03-06)

Adds support for CORS policy and HTTP request headers manipulation

#### Features

- CORS policy support [#83](https://github.com/fluxcd/flagger/pull/83)
- Allow headers to be appended to HTTP requests [#82](https://github.com/fluxcd/flagger/pull/82)

#### Improvements 

- Refactor the routing management 
    [#72](https://github.com/fluxcd/flagger/pull/72) 
    [#80](https://github.com/fluxcd/flagger/pull/80)
- Fine-grained RBAC [#73](https://github.com/fluxcd/flagger/pull/73)
- Add option to limit Flagger to a single namespace [#78](https://github.com/fluxcd/flagger/pull/78)

## 0.7.0 (2019-02-28)

Adds support for custom metric checks, HTTP timeouts and HTTP retries

#### Features

- Allow custom promql queries in the canary analysis spec [#60](https://github.com/fluxcd/flagger/pull/60)
- Add HTTP timeout and retries to canary service spec [#62](https://github.com/fluxcd/flagger/pull/62)

## 0.6.0 (2019-02-25)

Allows for [HTTPMatchRequests](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#HTTPMatchRequest) 
and [HTTPRewrite](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#HTTPRewrite) 
to be customized in the service spec of the canary custom resource.

#### Features

- Add HTTP match conditions and URI rewrite to the canary service spec [#55](https://github.com/fluxcd/flagger/pull/55)
- Update virtual service when the canary service spec changes 
    [#54](https://github.com/fluxcd/flagger/pull/54)
    [#51](https://github.com/fluxcd/flagger/pull/51)

#### Improvements 

- Run e2e testing on [Kubernetes Kind](https://github.com/kubernetes-sigs/kind) for canary promotion 
    [#53](https://github.com/fluxcd/flagger/pull/53)

## 0.5.1 (2019-02-14)

Allows skipping the analysis phase to ship changes directly to production

#### Features

- Add option to skip the canary analysis [#46](https://github.com/fluxcd/flagger/pull/46)

#### Fixes

- Reject deployment if the pod label selector doesn't match `app: <DEPLOYMENT_NAME>` [#43](https://github.com/fluxcd/flagger/pull/43)

## 0.5.0 (2019-01-30)

Track changes in ConfigMaps and Secrets [#37](https://github.com/fluxcd/flagger/pull/37)

#### Features

- Promote configmaps and secrets changes from canary to primary
- Detect changes in configmaps and/or secrets and (re)start canary analysis
- Add configs checksum to Canary CRD status
- Create primary configmaps and secrets at bootstrap
- Scan canary volumes and containers for configmaps and secrets

#### Fixes

- Copy deployment labels from canary to primary at bootstrap and promotion

## 0.4.1 (2019-01-24)

Load testing webhook [#35](https://github.com/fluxcd/flagger/pull/35)

#### Features

- Add the load tester chart to Flagger Helm repository
- Implement a load test runner based on [rakyll/hey](https://github.com/rakyll/hey)
- Log warning when no values are found for Istio metric due to lack of traffic

#### Fixes

- Run wekbooks before the metrics checks to avoid failures when using a load tester

## 0.4.0 (2019-01-18)

Restart canary analysis if revision changes [#31](https://github.com/fluxcd/flagger/pull/31)

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

Configurable canary analysis duration [#20](https://github.com/fluxcd/flagger/pull/20)

#### Breaking changes

- Helm chart: flag `controlLoopInterval` has been removed

#### Features

- CRD: canaries.flagger.app v1alpha3
- Schedule canary analysis independently based on `canaryAnalysis.interval`
- Add analysis interval to Canary CRD (defaults to one minute)
- Make autoscaler (HPA) reference optional

## 0.2.0 (2019-01-04)

Webhooks [#18](https://github.com/fluxcd/flagger/pull/18)

#### Features

- CRD: canaries.flagger.app v1alpha2
- Implement canary external checks based on webhooks HTTP POST calls
- Add webhooks to Canary CRD
- Move docs to gitbook [docs.flagger.app](https://docs.flagger.app)

## 0.1.2 (2018-12-06)

Improve Slack notifications [#14](https://github.com/fluxcd/flagger/pull/14)

#### Features

- Add canary analysis metadata to init and start Slack messages
- Add rollback reason to failed canary Slack messages

## 0.1.1 (2018-11-28)

Canary progress deadline [#10](https://github.com/fluxcd/flagger/pull/10)

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
