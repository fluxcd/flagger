# Flagger Kustomize installer

As an alternative to Helm, Flagger can be installed with [Kustomize](https://kustomize.io/).

## Service mesh specific installers

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

## Generic installer

Install Flagger and Prometheus:

```bash
kubectl apply -k github.com/weaveworks/flagger//kustomize/kubernetes
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

## Configure Slack notifications

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

Install Flagger for Istio with Slack notifications:

```bash
kubectl apply -k .
```

## Configure MS Teams notifications

Create a kustomization file using flagger as base:

```bash
cat > kustomization.yaml <<EOF
namespace: linkerd
bases:
  - github.com/weaveworks/flagger/kustomize/base/flagger
patchesStrategicMerge:
  - patch.yaml
EOF
```

Create a patch and set the MS Teams webhook URL:

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
            - -mesh-provider=linkerd
            - -metrics-server=http://linkerd-prometheus:9090
            - -msteams-url=https://outlook.office.com/webhook/YOUR/TEAMS/WEBHOOK
EOF
```

Install Flagger for Linkerd with MS Teams notifications:

```bash
kubectl apply -k .
```