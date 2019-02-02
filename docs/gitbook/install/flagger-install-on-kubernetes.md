# Flagger install on Kubernetes

This guide walks you through setting up Flagger on a Kubernetes cluster.

### Prerequisites

Flagger requires a Kubernetes cluster **v1.11** or newer with the following admission controllers enabled:

* MutatingAdmissionWebhook
* ValidatingAdmissionWebhook 

Flagger depends on [Istio](https://istio.io/docs/setup/kubernetes/quick-start/) **v1.0.3** or newer 
with traffic management, telemetry and Prometheus enabled. 

A minimal Istio installation should contain the following services:

* istio-pilot
* istio-ingressgateway
* istio-sidecar-injector
* istio-telemetry
* prometheus

### Install Flagger

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Deploy Flagger in the _**istio-system**_ namespace:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090
```

You can install Flagger in any namespace as long as it can talk to the Istio Prometheus service on port 9090.

Enable **Slack** notifications:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
--set slack.channel=general \
--set slack.user=flagger
```

If you don't have Tiller you can use the helm template command and apply the generated yaml with kubectl:

```bash
# generate
helm template flagger/flagger \
--name flagger \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 \
> $HOME/flagger.yaml

# apply
kubectl apply -f $HOME/flagger.yaml
```

### Install Grafana

Flagger comes with a Grafana dashboard made for monitoring the canary analysis.

Deploy Grafana in the _**istio-system**_ namespace:

```bash
helm upgrade -i flagger-grafana flagger/grafana \
--namespace=istio-system \
--set url=http://prometheus.istio-system:9090 \
--set user=admin \
--set password=change-me
```

Or use helm template command and apply the generated yaml with kubectl:

```bash
# generate
helm template flagger/grafana \
--name flagger-grafana \
--namespace=istio-system \
--set url=http://prometheus.istio-system:9090 \
--set user=admin \
--set password=change-me \
> $HOME/flagger-grafana.yaml

# apply
kubectl apply -f $HOME/flagger-grafana.yaml
```

You can access Grafana using port forwarding:

```bash
kubectl -n istio-system port-forward svc/flagger-grafana 3000:3000
```

###  Uninstall Flagger and Grafana

To uninstall the Flagger release with Helm run:

```text
helm delete --purge flagger
```

> **Note** that on uninstall the Canary CRD will not be removed. 
Deleting the CRD will make Kubernetes remove all the objects owned by Flagger like Istio virtual services, 
Kubernetes deployments and ClusterIP services.

If you want to remove all the objects created by Flagger you have delete the Canary CRD with kubectl:

```text
kubectl delete crd canaries.flagger.app
```

To uninstall the Grafana release with Helm run:

```text
helm delete --purge flagger-grafana
```

The command removes all the Kubernetes components associated with the chart and deletes the release.
