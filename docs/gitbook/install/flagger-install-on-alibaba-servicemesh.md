# Flagger Install on Alibaba ServiceMesh

This guide walks you through setting up Flagger on Alibaba ServiceMesh.

## Prerequisites
- Created an ACK([Alibabacloud Container Service for Kubernetes](https://cs.console.aliyun.com)) cluster instance.
- Create an ASM([Alibaba ServiceMesh](https://servicemesh.console.aliyun.com)) enterprise instance and add ACK cluster.

### Variables declaration
- `$ACK_CONFIG`: the kubeconfig file path of ACK, which be treated as`$HOME/.kube/config` in the rest of guide.
- `$MESH_CONFIG`: the kubeconfig file path of ASM.

### Enable Data-plane KubeAPI access in ASM

In the Alibaba Cloud Service Mesh (ASM) console, on the basic information page, make sure Data-plane KubeAPI access is enabled. When enabled, the Istio resources of the control plane can be managed through the Kubeconfig of the data plane cluster.

## Enable Prometheus

In the Alibaba Cloud Service Mesh (ASM) console, click  Settings to enable the collection of Prometheus monitoring metrics. You can use the self-built Prometheus monitoring, or you can use the Alibaba Cloud ARMS Prometheus monitoring plug-in that has joined the ACK cluster, and use ARMS Prometheus to collect monitoring indicators.

## Install Flagger

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Install Flagger's Canary CRD:

```bash
kubectl apply -f https://raw.githubusercontent.com/fluxcd/flagger/v1.21.0/artifacts/flagger/crd.yaml
```
## Deploy Flagger for Istio

### Add data plane cluster to Alibaba Cloud Service Mesh (ASM)

In the Alibaba Cloud Service Mesh (ASM) console, click Cluster & Workload Management, select the Kubernetes cluster, select the target ACK cluster, and add it to ASM.

### Prometheus address

If you are using Alibaba Cloud Container Service for Kubernetes (ACK) ARMS Prometheus monitoring, replace {Region-ID} in the link below with your region ID, such as cn-hangzhou. {ACKID} is the ACK ID of the data plane cluster that you added to Alibaba Cloud Service Mesh (ASM). Visit the following links to query the public and intranet addresses monitored by ACK's ARMS Prometheus:
[https://arms.console.aliyun.com/#/promDetail/{Region-ID}/{ACK-ID}/setting](https://arms.console.aliyun.com/)

An example of an intranet address is as follows:
[http://{Region-ID}-intranet.arms.aliyuncs.com:9090/api/v1/prometheus/{Prometheus-ID}/{u-id}/{ACK-ID}/{Region-ID}](https://arms.console.aliyun.com/)

## Deploy Flagger
Replace the value of metricsServer with your Prometheus address.

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090
```