# Flagger Install on EKS App Mesh

This guide walks you through setting up Flagger and AWS App Mesh on EKS.

## App Mesh

The App Mesh integration with EKS is made out of the following components:

* Kubernetes custom resources
  * `mesh.appmesh.k8s.aws` defines a logical boundary for network traffic between the services 
  * `virtualnode.appmesh.k8s.aws` defines a logical pointer to a Kubernetes workload
  * `virtualservice.appmesh.k8s.aws` defines the routing rules for a workload inside the mesh
* CRD controller - keeps the custom resources in sync with the App Mesh control plane
* Admission controller - injects the Envoy sidecar and assigns Kubernetes pods to App Mesh virtual nodes
* Telemetry service - Prometheus instance that collects and stores Envoy's metrics

## Create a Kubernetes cluster

In order to create an EKS cluster you can use [eksctl](https://eksctl.io).
Eksctl is an open source command-line utility made by Weaveworks in collaboration with Amazon.

On MacOS you can install eksctl with Homebrew:

```bash
brew tap weaveworks/tap
brew install weaveworks/tap/eksctl
```

Create an EKS cluster with:

```bash
eksctl create cluster --name=appmesh \
--region=us-west-2 \
--nodes 3 \
--node-volume-size=120 \
--appmesh-access
```

The above command will create a two nodes cluster with
App Mesh [IAM policy](https://docs.aws.amazon.com/app-mesh/latest/userguide/MESH_IAM_user_policies.html)
attached to the EKS node instance role.

Verify the install with:

```bash
kubectl get nodes
```

## Install Helm

Install the [Helm](https://docs.helm.sh/using_helm/#installing-helm) v3 command-line tool:

```text
brew install helm
```

Add the EKS repository to Helm:

```bash
helm repo add eks https://aws.github.io/eks-charts
```

## Enable horizontal pod auto-scaling

Install the Horizontal Pod Autoscaler (HPA) metrics provider:

```bash
helm upgrade -i metrics-server stable/metrics-server \
--namespace kube-system \
--set args[0]=--kubelet-preferred-address-types=InternalIP
```

After a minute, the metrics API should report CPU and memory usage for pods. You can very the metrics API with:

```bash
kubectl -n kube-system top pods
```

## Install the App Mesh components

Install the App Mesh CRDs:

```bash
kubectl apply -k github.com/aws/eks-charts/stable/appmesh-controller//crds?ref=master
```

Create the `appmesh-system` namespace:

```bash
kubectl create ns appmesh-system
```

Install the App Mesh controller:

```bash
helm upgrade -i appmesh-controller eks/appmesh-controller \
--wait --namespace appmesh-system
```

In order to collect the App Mesh metrics that Flagger needs to run the canary analysis,
you'll need to setup a Prometheus instance to scrape the Envoy sidecars.

Install the App Mesh Prometheus:

```bash
helm upgrade -i appmesh-prometheus eks/appmesh-prometheus \
--wait --namespace appmesh-system
```

## Install Flagger

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Install Flagger's Canary CRD:

```yaml
kubectl apply -f https://raw.githubusercontent.com/fluxcd/flagger/main/artifacts/flagger/crd.yaml
```

Deploy Flagger in the _**appmesh-system**_ namespace:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=appmesh-system \
--set crd.create=false \
--set meshProvider=appmesh:v1beta2 \
--set metricsServer=http://appmesh-prometheus:9090
```

## Install Grafana

Deploy App Mesh Grafana that comes with a dashboard for monitoring Flagger's canary releases:

```bash
helm upgrade -i appmesh-grafana eks/appmesh-grafana \
--namespace appmesh-system
```

You can access Grafana using port forwarding:

```bash
kubectl -n appmesh-system port-forward svc/appmesh-grafana 3000:3000
```

Now that you have Flagger running, you can try the
[App Mesh canary deployments tutorial](https://docs.flagger.app/usage/appmesh-progressive-delivery).

