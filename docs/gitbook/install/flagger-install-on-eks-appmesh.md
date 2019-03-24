# Flagger install on AWS

This guide walks you through setting up Flagger and AWS App Mesh on EKS.

### App Mesh

The App Mesh integration with EKS is made out of the following components:

* Kubernetes custom resources
    * `mesh.appmesh.k8s.aws` defines a logical boundary for network traffic between the services 
    * `virtualnode.appmesh.k8s.aws` defines a logical pointer to a Kubernetes workload
    * `virtualservice.appmesh.k8s.aws` defines the routing rules for a workload inside the mesh
* CRD controller - keeps the custom resources in sync with the App Mesh control plane
* Admission controller - injects the Envoy sidecar and assigns Kubernetes pods to App Mesh virtual nodes
* Metrics server - Prometheus instance that collects and stores Envoy's metrics

Prerequisites:

* homebrew
* openssl
* kubectl
* AWS CLI (default region us-west-2)

### Create a Kubernetes cluster

In order to create an EKS cluster you can use [eksctl](https://eksctl.io).
Eksctl is an open source command-line utility made by Weaveworks in collaboration with Amazon, 
it's written in Go and is based on EKS CloudFormation templates.

On MacOS you can install eksctl with Homebrew:

```bash
brew tap weaveworks/tap
brew install weaveworks/tap/eksctl
```

Create an EKS cluster:

```bash
eksctl create cluster --name=appmesh \
--region=us-west-2 \
--appmesh-access
```

The above command will create a two nodes cluster with App Mesh
[IAM policy](https://docs.aws.amazon.com/app-mesh/latest/userguide/MESH_IAM_user_policies.html)
attached to the EKS node instance role.

Verify the install with:

```bash
kubectl get nodes
```

### Install Helm

Install the [Helm](https://docs.helm.sh/using_helm/#installing-helm) command-line tool:

```text
brew install kubernetes-helm
```

Create a service account and a cluster role binding for Tiller:

```bash
kubectl -n kube-system create sa tiller

kubectl create clusterrolebinding tiller-cluster-rule \
--clusterrole=cluster-admin \
--serviceaccount=kube-system:tiller 
```

Deploy Tiller in the `kube-system` namespace:

```bash
helm init --service-account tiller
```

You should consider using SSL between Helm and Tiller, for more information on securing your Helm 
installation see [docs.helm.sh](https://docs.helm.sh/using_helm/#securing-your-helm-installation).

### Enable horizontal pod auto-scaling

Install the Horizontal Pod Autoscaler (HPA) metrics provider:

```bash
helm upgrade -i metrics-server stable/metrics-server \
--namespace kube-system
```

After a minute, the metrics API should report CPU and memory usage for pods.
You can very the metrics API with:

```bash
kubectl -n kube-system top pods
```

### Install the App Mesh components

Clone the config repo:

```bash
git clone https://github.com/stefanprodan/appmesh-eks
cd appmesh-eks
```

Create the `appmesh-system` namespace:

```bash
kubectl apply -f /namespaces/appmesh-system.yaml
```

Deploy the App Mesh Kubernetes CRDs and controller:

```bash
kubectl apply -f ./operator/
```

Install the App Mesh sidecar injector in the `appmesh-system` namespace:

```bash
./injector/install.sh
```

The above script generates a certificate signed by Kubernetes CA,
registers the App Mesh mutating webhook and deploys the injector.

Create a mesh called global in the `appmesh-system` namespace:

```bash
kubectl apply -f ./appmesh/global.yaml
```

Verify that the global mesh is active:

```bash
kubectl -n appmesh-system describe mesh

Status:
  Mesh Condition:
    Status:                True
    Type:                  MeshActive
```

### Install Prometheus

In order to expose the App Mesh metrics to Flagger, 
you'll need to use Prometheus to scrapes the Envoy sidecars.

Deploy Prometheus in the `appmesh-system` namespace:

```bash
REPO=https://raw.githubusercontent.com/weaveworks/flagger/master

kubectl apply -f ${REPO}/artifacts/eks/appmesh-prometheus.yaml
```

### Install Flagger and Grafana

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Deploy Flagger in the _**appmesh-system**_ namespace:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=appmesh-system \
--set meshProvider=appmesh \
--set metricsServer=http://prometheus.appmesh-system:9090
```

You can install Flagger in any namespace as long as it can talk to the Istio Prometheus service on port 9090.

You can enable **Slack** notifications with:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=appmesh-system \
--set meshProvider=appmesh \
--set metricsServer=http://prometheus.appmesh:9090 \
--set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
--set slack.channel=general \
--set slack.user=flagger
```

Flagger comes with a Grafana dashboard made for monitoring the canary analysis.
Deploy Grafana in the _**appmesh-system**_ namespace:

```bash
helm upgrade -i flagger-grafana flagger/grafana \
--namespace=appmesh-system \
--set url=http://prometheus.appmesh-system:9090
```

You can access Grafana using port forwarding:

```bash
kubectl -n appmesh-system port-forward svc/flagger-grafana 3000:80
```

###  Install the load tester

Flagger comes with an optional load testing service that generates traffic 
during canary analysis when configured as a webhook.

Create a test namespace with sidecar injector enabled:

```bash
kubectl apply -f ./namespaces/test.yaml
```

Deploy the load test runner with Helm:

```bash
helm upgrade -i flagger-loadtester flagger/loadtester \
--namespace=test \
--set meshName=global.appmesh-system \
--set backends[0]=frontend.test \
--set backends[1]=backend.test
```
