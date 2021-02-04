# Alibaba Cloud Service Mesh Canary Deployments

This guide shows you how to use Alibaba Cloud Service Mesh(ASM) and Flagger to automate canary deployments.
You'll need an ACK(Alibabacloud Container Service for Kubernetes) cluster (Kubernetes >= 1.16) configured with ASM(Alibaba Cloud Service Mesh), you can find the installation guide [here](https://docs.flagger.app/install/flagger-install-on-alibabacloud-servicemesh).

## Prerequisites

- Created an ACK([Alibabacloud Container Service for Kubernetes](https://cs.console.aliyun.com)) cluster instance.
- Created an ASM([Alibaba Cloud Service Mesh](https://servicemesh.console.aliyun.com)) instance, and added ACK cluster.

### Variables declaration

- `$ACK_CONFIG`: the kubeconfig file path of ACK, which be treated as`$HOME/.kube/config` in the rest of guide.
- `$MESH_CONFIG`: the kubeconfig file path of ASM.
- `$ISTIO_RELEASE`: see https://github.com/istio/istio/releases
- `$FLAGGER_SRC`: see https://github.com/fluxcd/flagger

## Bootstrap

### Setup EnvoyFilters for Metrics Monitoring
To automate canary deployments, Flagger uses the metrics from Prometheus, which are generated from Data Plane of Service Mesh, and configured by EnvoyFilters.

These EnvoyFilters could be generated automatically, if we choose one of the below options :
- Enable `Telemetry` using [UpdateMeshFeature](https://www.alibabacloud.com/help/doc-detail/171592.htm) API
- Enable Metrics Monitoring from [ASM Console](https://servicemesh.console.aliyun.com/)

### Setup Gateway
Create an ingress gateway to expose the demo app outside of the mesh:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: public-gateway
  namespace: istio-system
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 80
        name: http
        protocol: HTTP
      hosts:
        - "*"
```

### Initialize
Create a test namespace with Istio sidecar injection enabled:

```bash
kubectl create ns test
kubectl --kubeconfig $MESH_CONFIG create ns test
kubectl --kubeconfig $MESH_CONFIG label namespace test istio-injection=enabled
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/podinfo?ref=main
```

Deploy the load testing service to generate traffic during the canary analysis:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/tester?ref=main
```

Create a canary custom resource yaml:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  # deployment reference
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  # HPA reference (optional)
  autoscalerRef:
    apiVersion: autoscaling/v2beta2
    kind: HorizontalPodAutoscaler
    name: podinfo
  service:
    # service port number
    port: 9898
    # container port number or name (optional)
    targetPort: 9898
    # Istio gateways (optional)
    gateways:
    - public-gateway.istio-system.svc.cluster.local
    # Istio virtual service host names (optional)
    hosts:
    - '*'
    # Istio traffic policy (optional)
    trafficPolicy:
      tls:
        # use ISTIO_MUTUAL when mTLS is enabled
        mode: DISABLE
    # Istio retry policy (optional)
    retries:
      attempts: 3
      perTryTimeout: 1s
      retryOn: "gateway-error,connect-failure,refused-stream"
  analysis:
    # schedule interval (default 60s)
    interval: 1m
    # max number of failed metric checks before rollback
    threshold: 5
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 10
    metrics:
    - name: request-success-rate
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      # maximum req duration P99
      # milliseconds
      thresholdRange:
        max: 500
      interval: 30s
    # testing (optional)
    webhooks:
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary:9898/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test:9898/"
```

Save the above resource as podinfo-canary.yaml and then apply it:

```bash
kubectl apply -f podinfo-canary.yaml
```

## Automated canary promotion

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=stefanprodan/podinfo:3.1.1
```

Flagger detects that the deployment revision changed and starts a new rollout. Watch the Events of the canary:

```bash
while true; do kubectl -n test describe canary/podinfo; sleep 10s;done
```