# Flagger Install on Kubernetes with Flux

This guide walks you through setting up Flagger on a Kubernetes cluster the GitOps way.
You'll configure Flux to scan the Flagger OCI artifacts and deploy the
latest stable version on Kubernetes.

## Flagger OCI artifacts

Flagger OCI artifacts (container images, Helm charts, Kustomize overlays) are published to
GitHub Container Registry, and they are signed with Cosign at every release.

OCI artifacts

- `ghcr.io/fluxcd/flagger:<version>` multi-arch container images
- `ghcr.io/fluxcd/flagger-manifest:<version>` Kubernetes manifests
- `ghcr.io/fluxcd/charts/flagger:<version>` Helm charts

## Prerequisites

To follow this guide you’ll need a Kubernetes cluster with Flux installed on it.
Please see the Flux [get started guide](https://fluxcd.io/flux/get-started/)
or the Flux [installation guide](https://fluxcd.io/flux/installation/).

## Deploy Flagger with Flux

First define the namespace where Flagger will be installed:

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: flagger-system
  labels:
    toolkit.fluxcd.io/tenant: sre-team
```

Define a Flux `HelmRepository` that points to where the Flagger Helm charts are stored:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: flagger
  namespace: flagger-system
spec:
  interval: 1h
  url: oci://ghcr.io/fluxcd/charts
  type: oci
```

Define a Flux `HelmRelease` that verifies and installs Flagger's latest version on the cluster:

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: flagger
  namespace: flagger-system
spec:
  interval: 1h
  releaseName: flagger
  install: # override existing Flagger CRDs
    crds: CreateReplace
  upgrade: # update Flagger CRDs
    crds: CreateReplace
  chart:
    spec:
      chart: flagger
      version: 1.x # update Flagger to the latest minor version
      interval: 6h # scan for new versions every six hours
      sourceRef:
        kind: HelmRepository
        name: flagger
      verify: # verify the chart signature with Cosign keyless
        provider: cosign 
  values:
    nodeSelector:
      kubernetes.io/os: linux
```

Copy the above manifests into a file called `flagger.yaml`, place the YAML file
in the Git repository bootstrapped with Flux, then commit and push it to upstream.

After Flux reconciles the changes on your cluster, you can check if Flagger got deployed with:

```console
$ helm list -n flagger-system 
NAME    NAMESPACE       REVISION        STATUS          CHART           APP VERSION
flagger flagger-system  1               deployed        flagger-1.23.0  1.23.0  
```

To uninstall Flagger, delete the `flagger.yaml` from your repository, then Flux will uninstall
the Helm release and will remove the namespace from your cluster.

## Deploy Flagger load tester with Flux

Flagger comes with a load testing service that generates traffic during analysis when configured as a webhook.

The load tester container images and deployment manifests are published to GitHub Container Registry.
The container images and the manifests are signed with Cosign and GitHub Actions OIDC.

Assuming the applications managed by Flagger are in the `apps` namespace, you can configure Flux to
deploy the load tester there.

Define a Flux `OCIRepository` that points to where the Flagger Kustomize overlays are stored:

```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: flagger-loadtester
  namespace: apps
spec:
  interval: 6h # scan for new versions every six hours
  url: oci://ghcr.io/fluxcd/flagger-manifests
  ref:
    semver: 1.x # update to the latest version 
  verify: # verify the artifact signature with Cosign keyless
    provider: cosign
```

Define a Flux `Kustomization` that deploys the Flagger load tester to the `apps` namespace:

```yaml
---
apiVersion: kustomize.toolkit.fluxcd.io/v1beta2
kind: Kustomization
metadata:
  name: flagger-loadtester
  namespace: apps
spec:
  interval: 6h
  wait: true
  timeout: 5m
  prune: true
  sourceRef:
    kind: OCIRepository
    name: flagger-loadtester
  path: ./tester
  targetNamespace: apps
```

Copy the above manifests into a file called `flagger-loadtester.yaml`, place the YAML file
in the Git repository bootstrapped with Flux, then commit and push it to upstream.

After Flux reconciles the changes on your cluster, you can check if the load tester got deployed with:

```console
$ flux -n apps get kustomization flagger-loadtester 
NAME              	READY	MESSAGE                                                                                    
flagger-loadtester	True 	Applied revision: v1.23.0/a80af71e001
```

To uninstall the load tester, delete the `flagger-loadtester.yaml` from your repository, 
and Flux will delete the load tester deployment from the cluster.
