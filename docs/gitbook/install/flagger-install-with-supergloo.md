# Flagger install on Kubernetes with SuperGloo

This guide walks you through setting up Flagger on a Kubernetes cluster using [SuperGloo](https://github.com/solo-io/supergloo).

SuperGloo by [Solo.io](https://solo.io) is an opinionated abstraction layer that will simplify the installation, management, and operation of your service mesh. 
It supports running multiple ingress with multiple mesh (Istio, App Mesh, Consul Connect and Linkerd 2) in the same cluster. 

### Prerequisites

Flagger requires a Kubernetes cluster **v1.11** or newer with the following admission controllers enabled:

* MutatingAdmissionWebhook
* ValidatingAdmissionWebhook 

### Install Istio with SuperGloo

Download SuperGloo CLI and add it to your path:

```bash
curl -sL https://run.solo.io/supergloo/install | sh
export PATH=$HOME/.supergloo/bin:$PATH
```

Deploy the SuperGloo controller in the `supergloo-system` namespace:

```bash
supergloo init
```

Create the `istio-system` namespace and install Istio with traffic management, telemetry and Prometheus enabled:

```bash
ISTIO_VER="1.0.6"

kubectl create ns istio-system

supergloo install istio --name istio \
--namespace=supergloo-system \
--auto-inject=true \
--installation-namespace=istio-system \
--mtls=false \
--prometheus=true \
--version ${ISTIO_VER}
```

Create a cluster role binding so that Flagger can manipulate SuperGloo custom resources:

```bash
kubectl create clusterrolebinding flagger-supergloo \
--clusterrole=mesh-discovery \
--serviceaccount=istio-system:flagger
```

Wait for the Istio control plane to become available:

```bash
kubectl -n istio-system rollout status deployment/istio-sidecar-injector
kubectl -n istio-system rollout status deployment/prometheus
```

### Install Flagger

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Deploy Flagger in the _**istio-system**_ namespace and set the service mesh provider to SuperGloo:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
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

###  Install Load Tester

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
