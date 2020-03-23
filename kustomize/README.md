# Flagger Kustomize installer

As an alternative to Helm, Flagger can be installed with [Kustomize](https://kustomize.io/).

**Prerequisites**

- Kubernetes cluster **>=1.13.0**
- Kustomize **>=3.5.0**

## Service mesh specific installers

Install Flagger for Istio:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/istio | kubectl apply -f -
```

This deploys Flagger in the `istio-system` namespace and sets the metrics server URL to Istio's Prometheus instance.

Install Flagger for AWS App Mesh:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/appmesh | kubectl apply -f -
```

This deploys Flagger in the `appmesh-system` namespace and sets the metrics server URL to App Mesh Prometheus instance.

Install Flagger for Linkerd:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/linkerd | kubectl apply -f -
```

This deploys Flagger in the `linkerd` namespace and sets the metrics server URL to Linkerd's Prometheus instance.

If you want to install a specific Flagger release, add the version number to the URL:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/linkerd?ref=0.18.0 | kubectl apply -f -
```

Install Flagger for Contour:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/contour | kubectl apply -f -
```

This deploys Flagger and Prometheus in the `projectcontour` namespace and sets Prometheus to scrape Contour's Envoy instances.

## Generic installer

Install Flagger and Prometheus:

```bash
kustomize build github.com/weaveworks/flagger//kustomize/kubernetes | kubectl apply -f -
```

This deploys Flagger and Prometheus in the `flagger-system` namespace,
sets the metrics server URL to `http://flagger-prometheus.flagger-system:9090` and the mesh provider to `kubernetes`.

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

You'll need Prometheus when using Flagger with AWS App Mesh, Gloo or NGINX ingress controller.
The Prometheus instance has a two hours data retention and is configured to scrape all pods in your cluster that
have the `prometheus.io/scrape: "true"` annotation.

## Customise the installation

Create a kustomization file using Flagger as base and patch the container args:

```bash
cat > kustomization.yaml <<EOF
namespace: istio-system
bases:
  - github.com/weaveworks/flagger/kustomize/base/flagger
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
              - -slack-user=flagger
              - -slack-channel=alerts
              - -slack-url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK
EOF
```

Install Flagger for Istio with Slack notifications:

```bash
kustomize build . | kubectl apply -f -
```
