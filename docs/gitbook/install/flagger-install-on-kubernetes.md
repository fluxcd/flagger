# Flagger Install on Kubernetes

This guide walks you through setting up Flagger on a Kubernetes cluster with Helm or Kubectl.

See the [Flux install guide](flagger-install-with-flux.md) for installing Flagger and keeping it up to date the GitOps way.

## Install Flagger with Helm

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Install Flagger's Canary CRD:

```yaml
kubectl apply -f https://raw.githubusercontent.com/fluxcd/flagger/main/artifacts/flagger/crd.yaml
```

Deploy Flagger for Istio:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090
```

Note that Flagger depends on Istio telemetry and Prometheus, if you're installing
Istio with istioctl then you should be using the
[default profile](https://istio.io/docs/setup/additional-setup/config-profiles/).

For Istio multi-cluster shared control plane you can install Flagger on each remote cluster and set the
Istio control plane host cluster kubeconfig:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://istio-cluster-prometheus:9090 \
--set controlplane.kubeconfig.secretName=istio-kubeconfig \
--set controlplane.kubeconfig.key=kubeconfig
```

Note that the Istio kubeconfig must be stored in a Kubernetes secret with a data key named `kubeconfig`.
For more details on how to configure Istio multi-cluster
credentials read the [Istio docs](https://istio.io/docs/setup/install/multicluster/shared-vpn/#credentials).

Deploy Flagger for Linkerd:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=linkerd \
--set crd.create=false \
--set meshProvider=linkerd \
--set metricsServer=http://linkerd-prometheus:9090
```

If you need to add labels to the flagger deployment or pods, you can pass the labels as parameters as shown below.

```console
helm upgrade -i flagger flagger/flagger \
<other parameters> \
--set podLabels.<labelName>=<labelValue> \
--set deploymentLabels.<labelName>=<labelValue> 
```

You can install Flagger in any namespace as long as it can talk to the Prometheus service on port 9090.

For ingress controllers, the installation instructions are:

* [Contour](https://docs.flagger.app/tutorials/contour-progressive-delivery)
* [Gloo](https://docs.flagger.app/tutorials/gloo-progressive-delivery)
* [NGINX](https://docs.flagger.app/tutorials/nginx-progressive-delivery)
* [Skipper](https://docs.flagger.app/tutorials/skipper-progressive-delivery)
* [Traefik](https://docs.flagger.app/tutorials/traefik-progressive-delivery)
* [APISIX](https://docs.flagger.app/tutorials/apisix-progressive-delivery)

To uninstall the Flagger release with Helm run:

```text
helm delete flagger
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

> **Note** that on uninstall the Canary CRD will not be removed. Deleting the CRD will make Kubernetes
> remove all the objects owned by Flagger like Istio virtual services, Kubernetes deployments and ClusterIP services.

If you want to remove all the objects created by Flagger you have to delete the Canary CRD with kubectl:

```text
kubectl delete crd canaries.flagger.app
```

## Install Grafana with Helm

Flagger comes with a Grafana dashboard made for monitoring the canary analysis.

Deploy Grafana in the _**istio-system**_ namespace:

```bash
helm upgrade -i flagger-grafana flagger/grafana \
--namespace=istio-system \
--set url=http://prometheus.istio-system:9090 \
--set user=admin \
--set password=change-me
```

You can access Grafana using port forwarding:

```bash
kubectl -n istio-system port-forward svc/flagger-grafana 3000:80
```

## Install Flagger with Kubectl

Install Flagger and Prometheus using the Kustomize overlay from the GitHub repository:

```bash
kubectl apply -k https://github.com/fluxcd/flagger/kustomize/kubernetes?ref=main
```

This deploys Flagger and Prometheus in the `flagger-system` namespace,
sets the metrics server URL to `http://flagger-prometheus.flagger-system:9090` and the mesh provider to `kubernetes`.

The Prometheus instance has a two hours data retention and is configured to scrape all pods in your cluster
that have the `prometheus.io/scrape: "true"` annotation.

**Customized installer**

Create a kustomization file using Flagger as base and patch the container args:

```bash
cat > kustomization.yaml <<EOF
namespace: istio-system
bases:
  - https://github.com/fluxcd/flagger/kustomize/kubernetes?ref=main
patches:
- target:
    kind: Deployment
    name: flagger
  patch: |-
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
              - -include-label-prefix=app.kubernetes.io
              - -namespace=production,staging
EOF
```

## Namespace Configuration

By default, Flagger watches all namespaces for Canary resources. You can restrict Flagger to watch specific namespaces by setting the `--namespace` flag with a comma-separated list of namespaces:

**Single namespace:**
```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090 \
--set namespace=production
```

**Multiple namespaces:**
```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090 \
--set namespace=production,staging,testing
```

**Watch all namespaces (default):**
```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090
# No namespace flag means watch all namespaces
```
