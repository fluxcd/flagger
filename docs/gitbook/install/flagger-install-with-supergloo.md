# Flagger install on Kubernetes with SuperGloo

This guide walks you through setting up Flagger on a Kubernetes cluster using [SuperGloo](https://github.com/solo-io/supergloo).

SuperGloo by [Solo.io](https://solo.io) is an opinionated abstraction layer that simplifies the installation, management, and operation of your service mesh.
It supports running multiple ingresses with multiple meshes (Istio, App Mesh, Consul Connect and Linkerd 2) in the same cluster.

### Prerequisites

Flagger requires a Kubernetes cluster **v1.11** or newer with the following admission controllers enabled:

* MutatingAdmissionWebhook
* ValidatingAdmissionWebhook

### Install Istio with SuperGloo

#### Install SuperGloo command line interface helper

SuperGloo includes a command line helper (CLI) that makes operation of SuperGloo easier.
The CLI is not required for SuperGloo to function correctly.

If you use [Homebrew](https://brew.sh) package manager run the following
commands to install the SuperGloo CLI.

```bash
brew tap solo-io/tap
brew solo-io/tap/supergloo
```

Or you can download SuperGloo CLI and add it to your path:

```bash
curl -sL https://run.solo.io/supergloo/install | sh
export PATH=$HOME/.supergloo/bin:$PATH
```

#### Install SuperGloo controller

Deploy the SuperGloo controller in the `supergloo-system` namespace:

```bash
supergloo init
```

This is equivalent to installing SuperGloo using its Helm chart

```bash
helm repo add supergloo http://storage.googleapis.com/supergloo-helm
helm upgrade --install supergloo supergloo/supergloo --namespace supergloo-system
```

#### Install Istio using SuperGloo

Create the `istio-system` namespace and install Istio with traffic management, telemetry and Prometheus enabled:

```bash
ISTIO_VER="1.0.6"

kubectl create namespace istio-system

supergloo install istio --name istio \
--namespace=supergloo-system \
--auto-inject=true \
--installation-namespace=istio-system \
--mtls=false \
--prometheus=true \
--version=${ISTIO_VER}
```

This creates a Kubernetes Custom Resource (CRD) like the following.

```yaml
apiVersion: supergloo.solo.io/v1
kind: Install
metadata:
  name: istio
  namespace: supergloo-system
spec:
  installationNamespace: istio-system
  mesh:
    installedMesh:
      name: istio
      namespace: supergloo-system
    istioMesh:
      enableAutoInject: true
      enableMtls: false
      installGrafana: false
      installJaeger: false
      installPrometheus: true
      istioVersion: 1.0.6
```

#### Allow Flagger to manipulate SuperGloo

Create a cluster role binding so that Flagger can manipulate SuperGloo custom resources:

```bash
kubectl create clusterrolebinding flagger-supergloo \
--clusterrole=mesh-discovery \
--serviceaccount=istio-system:flagger
```

Wait for the Istio control plane to become available:

```bash
kubectl --namespace istio-system rollout status deployment/istio-sidecar-injector
kubectl --namespace istio-system rollout status deployment/prometheus
```

### Install Flagger

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Install Flagger's Canary CRD:

```yaml
kubectl apply -f https://raw.githubusercontent.com/weaveworks/flagger/master/artifacts/flagger/crd.yaml
```

Deploy Flagger in the _**istio-system**_ namespace and set the service mesh provider to SuperGloo:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set metricsServer=http://prometheus.istio-system:9090 \
--set meshProvider=supergloo:istio.supergloo-system
```

When using SuperGloo the mesh provider format is `supergloo:<MESH-NAME>.<SUPERGLOO-NAMESPACE>`.

Optionally you can enable **Slack** notifications:

```bash
helm upgrade -i flagger flagger/flagger \
--reuse-values \
--namespace=istio-system \
--set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
--set slack.channel=general \
--set slack.user=flagger
```

### Install Grafana

Flagger comes with a Grafana dashboard made for monitoring the canary analysis.

Deploy Grafana in the _**istio-system**_ namespace:

```bash
helm upgrade -i flagger-grafana flagger/grafana \
--namespace=istio-system \
--set url=http://prometheus.istio-system:9090
```

You can access Grafana using port forwarding:

```bash
kubectl -n istio-system port-forward svc/flagger-grafana 3000:80
```

### Install Load Tester

Flagger comes with an optional load testing service that generates traffic
during canary analysis when configured as a webhook.

Deploy the load test runner with Helm:

```bash
helm upgrade -i flagger-loadtester flagger/loadtester \
--namespace=test \
--set cmd.timeout=1h
```

Deploy with kubectl:

```bash
helm fetch --untar --untardir . flagger/loadtester &&
helm template loadtester \
--name flagger-loadtester \
--namespace=test
> $HOME/flagger-loadtester.yaml

# apply
kubectl apply -f $HOME/flagger-loadtester.yaml
```

> **Note** that the load tester should be deployed in a namespace with Istio sidecar injection enabled.
