# Flagger install on Kubernetes

This guide walks you through setting up Flagger on a Kubernetes cluster with Helm or Kustomize.

### Prerequisites

Flagger requires a Kubernetes cluster **v1.11** or newer with the following admission controllers enabled:

* MutatingAdmissionWebhook
* ValidatingAdmissionWebhook 

### Install Flagger with Helm

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Install Flagger's Canary CRD:

```yaml
kubectl apply -f https://raw.githubusercontent.com/weaveworks/flagger/master/artifacts/flagger/crd.yaml
```

Deploy Flagger for Istio:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090
```

Deploy Flagger for Linkerd:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=linkerd \
--set crd.create=false \
--set meshProvider=linkerd \
--set metricsServer=http://linkerd-prometheus:9090
```

You can install Flagger in any namespace as long as it can talk to the Prometheus service on port 9090.

Enable **Slack** notifications:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
--set slack.channel=general \
--set slack.user=flagger
```

Enable **Microsoft Teams** notifications:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set msteams.url=https://outlook.office.com/webhook/YOUR/TEAMS/WEBHOOK
```

If you don't have Tiller you can use the helm template command and apply the generated yaml with kubectl:

```bash
# generate
helm fetch --untar --untardir . flagger/flagger &&
helm template flagger \
--name flagger \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 \
> $HOME/flagger.yaml

# apply
kubectl apply -f $HOME/flagger.yaml
```

To uninstall the Flagger release with Helm run:

```text
helm delete --purge flagger
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

> **Note** that on uninstall the Canary CRD will not be removed. 
Deleting the CRD will make Kubernetes remove all the objects owned by Flagger like Istio virtual services, 
Kubernetes deployments and ClusterIP services.

If you want to remove all the objects created by Flagger you have delete the Canary CRD with kubectl:

```text
kubectl delete crd canaries.flagger.app
```

### Install Grafana with Helm

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
helm fetch --untar --untardir . flagger/grafana &&
helm template grafana \
--name flagger-grafana \
--namespace=istio-system \
> $HOME/flagger-grafana.yaml

# apply
kubectl apply -f $HOME/flagger-grafana.yaml
```

You can access Grafana using port forwarding:

```bash
kubectl -n istio-system port-forward svc/flagger-grafana 3000:80
```

### Install Flagger with Kustomize

As an alternative to Helm, Flagger can be installed with Kustomize.

**Service mesh specific installers**

Install Flagger for Istio:

```bash
kubectl apply -k github.com/weaveworks/flagger//kustomize/istio
```

This deploys Flagger in the `istio-system` namespace and sets the metrics server URL to Istio's Prometheus instance.

Note that you'll need kubectl 1.14 to run the above the command or you can download the
[kustomize binary](https://github.com/kubernetes-sigs/kustomize/releases) and run:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/istio | kubectl apply -f -
```

Install Flagger for Linkerd:

```bash
kubectl apply -k github.com/weaveworks/flagger//kustomize/linkerd
```

This deploys Flagger in the `linkerd` namespace and sets the metrics server URL to Linkerd's Prometheus instance.

If you want to install a specific Flagger release, add the version number to the URL:

```bash
kubectl apply -k github.com/weaveworks/flagger//kustomize/linkerd?ref=0.18.0
```

**Generic installer**

Install Flagger and Prometheus:

```bash
kubectl apply -k github.com/weaveworks/flagger//kustomize/kubernetes
```

This deploys Flagger and Prometheus in the `flagger-system` namespace,
sets the metrics server URL to `http://flagger-prometheus.flagger-system:9090` and the mesh provider to `kubernetes`.

The Prometheus instance has a two hours data retention and is configured to scrape all pods in your cluster that
have the `prometheus.io/scrape: "true"` annotation.

To target a different provider you can specify it in the canary custom resource:

```yaml
apiVersion: flagger.app/v1alpha3
kind: Canary
metadata:
  name: app
  namespace: test
spec:
  # can be: kubernetes, istio, linkerd, appmesh, nginx, gloo
  # use the kubernetes provider for Blue/Green style deployments
  provider: nginx
```

**Customized installer**

Create a kustomization file using flagger as base:

```bash
cat > kustomization.yaml <<EOF
namespace: istio-system
bases:
  - github.com/weaveworks/flagger/kustomize/base/flagger
patchesStrategicMerge:
  - patch.yaml
EOF
```

Create a patch and enable Slack notifications by setting the slack channel and hook URL:

```bash
cat > patch.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flagger
spec:
  template:
    spec:
      containers:
        - name: flagger
          args:
            - -mesh-provider=istio
            - -metrics-server=http://prometheus.istio-system:9090
            - -slack-user=flagger
            - -slack-channel=alerts
            - -slack-url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK
EOF
```

Install Flagger with Slack:

```bash
kubectl apply -k .
```

If you want to use MS Teams instead of Slack, replace `-slack-url` with `-msteams-url` and set the webhook address to `https://outlook.office.com/webhook/YOUR/TEAMS/WEBHOOK`.
