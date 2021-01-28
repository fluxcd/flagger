# Flagger Install on Alibaba ServiceMesh

This guide walks you through setting up Flagger on Alibaba ServiceMesh.

## Prerequisites
- Created an ACK([Alibabacloud Container Service for Kubernetes](https://cs.console.aliyun.com)) cluster instance.
- Created an ASM([Alibaba ServiceMesh](https://servicemesh.console.aliyun.com)) instance, and added ACK cluster.

### Variables declaration
- `$ACK_CONFIG`: the kubeconfig file path of ACK, which be treated as`$HOME/.kube/config` in the rest of guide.
- `$MESH_CONFIG`: the kubeconfig file path of ASM.
- `$ISTIO_RELEASE`: see https://github.com/istio/istio/releases
- `$FLAGGER_SRC`: see https://github.com/fluxcd/flagger

## Install Prometheus
Install Prometheus:

```bash
kubectl apply -f $ISTIO_RELEASE/samples/addons/prometheus.yaml
```

it' same with the below cmd:

```bash
kubectl --kubeconfig "$ACK_CONFIG" apply -f $ISTIO_RELEASE/samples/addons/prometheus.yaml
```

Append the below configs to `scrape_configs` in prometheus configmap, to support telemetry:
```yaml
scrape_configs:
# Mixer scrapping. Defaults to Prometheus and mixer on same namespace.
- job_name: 'istio-mesh'
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - istio-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
    action: keep
    regex: istio-telemetry;prometheus
# Scrape config for envoy stats
- job_name: 'envoy-stats'
  metrics_path: /stats/prometheus
  kubernetes_sd_configs:
  - role: pod
  relabel_configs:
  - source_labels: [__meta_kubernetes_pod_container_port_name]
    action: keep
    regex: '.*-envoy-prom'
  - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
    action: replace
    regex: ([^:]+)(?::\d+)?;(\d+)
    replacement: $1:15090
    target_label: __address__
  - action: labeldrop
    regex: __meta_kubernetes_pod_label_(.+)
  - source_labels: [__meta_kubernetes_namespace]
    action: replace
    target_label: namespace
  - source_labels: [__meta_kubernetes_pod_name]
    action: replace
    target_label: pod_name
- job_name: 'istio-policy'
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - istio-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
    action: keep
    regex: istio-policy;http-policy-monitoring
- job_name: 'istio-telemetry'
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - istio-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
    action: keep
    regex: istio-telemetry;http-monitoring
- job_name: 'pilot'
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - istio-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
    action: keep
    regex: istiod;http-monitoring
  - source_labels: [__meta_kubernetes_service_label_app]
    target_label: app
- job_name: 'sidecar-injector'
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - istio-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
    action: keep
    regex: istio-sidecar-injector;http-monitoring
```

## Install Flagger

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
helm repo update
```

Install Flagger's Canary CRD:

```yaml
kubectl apply -f $FLAGGER_SRC/artifacts/flagger/crd.yaml
```

Deploy Flagger for Alibaba ServiceMesh:

```bash
cp $MESH_CONFIG kubeconfig
kubectl -n istio-system create secret generic istio-kubeconfig --from-file kubeconfig
kubectl -n istio-system label secret istio-kubeconfig istio/multiCluster=true
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set crd.create=false \
--set meshProvider=istio \
--set metricsServer=http://prometheus:9090 \
--set istio.kubeconfig.secretName=istio-kubeconfig \
--set istio.kubeconfig.key=kubeconfig
```
