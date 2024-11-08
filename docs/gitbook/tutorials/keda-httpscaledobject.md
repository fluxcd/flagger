# Canary analysis with KEDA HTTP Add-on HTTPScaledObjects

This guide shows you how to use Flagger with KEDA HTTPScaledObjects which will automatically scale (including to/from zero) based on incoming HTTP traffic a Canary analysis run.

![Flagger Canary Stages](https://github.com/fluxcd/flagger/blob/dbde37581e052e74e3456cafaf8ec37d3a9e8c77/docs/diagrams/flagger-keda-http-add-on.png)

## Prerequisites

Flagger requires a Kubernetes cluster **v1.19** or newer and any mesh/ingress that implements the `v1` version of Gateway API. For this tutorial, we'll need KEDA **2.16.0** or newer and KEDA HTTP Add-on **0.9.0** or newer.

Install the Gateway API CRDs:

```bash
kubectl apply -k "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v1.2.0"
```

Install Istio:

```bash
istioctl install --set profile=minimal -y

# Suggestion: Please change release-1.20 in below command, to your real istio version.
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/prometheus.yaml
```

Install KEDA:

```bash
helm repo add kedacore https://kedacore.github.io/charts
helm repo update
helm upgrade -i keda kedacore/keda --namespace keda --create-namespace --version 2.15.1
```

Install KEDA HTTP Add-on:

```bash
helm upgrade -i http-add-on oci://quay.io/kahirokunn/keda/keda-add-ons-http \
  --version 0.9.0 \
  -n keda \
  --create-namespace \
  --set images.tag=0.9.0 \
  --set images.operator=quay.io/kahirokunn/http-add-on-operator \
  --set images.interceptor=quay.io/kahirokunn/http-add-on-interceptor \
  --set images.scaler=quay.io/kahirokunn/http-add-on-scaler
```

Install Flagger:

```bash
helm repo add flagger https://flagger.app
helm repo update

helm upgrade -i flagger oci://quay.io/kahirokunn/flagger/flagger \
  --namespace flagger-system \
  --create-namespace \
  --set prometheus.install=false \
  --set meshProvider=gatewayapi:v1 \
  --set metricsServer=http://prometheus.istio-system:9090 \
  --set image.repository=quay.io/kahirokunn/flagger \
  --set image.tag=latest
```

> Note: The above installation sets the mesh provider to be `gatewayapi:v1`. If your Gateway API implementation uses the `v1beta1` CRDs, then
set the `--meshProvider` value to `gatewayapi:v1beta1`.

Create a namespace for the `Gateway`:

```bash
kubectl create ns istio-ingress
```

Create a `Gateway` that configures load balancing, traffic ACL, etc:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: gateway
  namespace: istio-ingress
spec:
  gatewayClassName: istio
  listeners:
  - name: default
    hostname: "*.example.com"
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
```

Create a interceptor for the primary workloads:

```yaml
kind: ClusterHTTPScalingSet
apiVersion: http.keda.sh/v1alpha1
metadata:
  name: primary
spec:
  interceptor:
    image: quay.io/kahirokunn/http-add-on-interceptor:0.9.0
    config:
      adminPort: 9090
      connectTimeout: 500ms
      expectContinueTimeout: 1s
      forceHTTP2: false
      handshakeTimeout: 10s
      headerTimeout: 500ms
      idleConnTimeout: 90s
      keepAlive: 1s
      maxIdleConnections: 100
      pollingInterval: 1000
      proxyPort: 8080
      waitTimeout: 20s
    replicas: 1
    resources: {}
    serviceAccountName: keda-add-ons-http-interceptor
  scaler:
    image: quay.io/kahirokunn/http-add-on-scaler:0.9.0
    serviceAccountName: keda-add-ons-http-external-scaler
```

## Bootstrap

Flagger takes a Kubernetes deployment and a KEDA HTTPScaledObject targeting the deployment. It then creates a series of objects
(Kubernetes deployments, ClusterIP services targeting the Deployments, another KEDA HTTPScaledObject targeting the created Deployment and ClusterIP service, HTTPRoute targeting the KEDA interceptor).
These objects expose the application inside the mesh and drive the Canary analysis and Blue/Green promotion.

Create a test namespace:

```bash
kubectl create ns test
```

Create a Deployment named `podinfo`:

```bash
kubectl apply -n test -f https://raw.githubusercontent.com/fluxcd/flagger/main/kustomize/podinfo/deployment.yaml
```

Create a ClusterIP Service and HTTPScaledObject which targets the `podinfo` deployment and uses HTTP Request as a trigger:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: podinfo
  namespace: test
  labels:
    app: podinfo
spec:
  type: ClusterIP
  ports:
    - port: 9898
      targetPort: http
      protocol: TCP
      name: http
  selector:
    app: podinfo
---
apiVersion: http.keda.sh/v1alpha1
kind: HTTPScaledObject
metadata:
  name: podinfo
  namespace: test
  labels:
    app: podinfo
spec:
  hosts:
    - www.example.com
  pathPrefixes:
    - /
  replicas:
    max: 10
    min: 0
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
    port: 9898
    service: podinfo
  scalingMetric:
    concurrency:
      targetValue: 200
```

Save the above resource as podinfo-canary-httpso.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary-httpso.yaml
```

Deploy the load testing service to generate traffic during the analysis:

```bash
kubectl apply -k 'https://github.com/fluxcd/flagger//kustomize/tester?ref=main'
```

Create a canary custom resource \(replace "www.example.com" with your own domain\):

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
  # HTTPScaledObject reference
  autoscalerRef:
    apiVersion: keda.sh/v1alpha1
    kind: HTTPScaledObject
    name: podinfo
    primaryScalingSet:
      name: primary
      kind: ClusterHTTPScalingSet
  # the maximum time in seconds for the canary deployment
  # to make progress before rollback (default 600s)
  progressDeadlineSeconds: 60
  service:
    # service port number
    port: 8080
    # Reference to the Gateway that the generated HTTPRoute would attach to.
    gatewayRefs:
      - name: gateway
        namespace: istio-ingress
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
    - name: error-rate
      # max error rate (5xx responses)
      # percentage (0-100)
      templateRef:
        name: error-rate
        namespace: flagger-system
      thresholdRange:
        max: 1
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: flagger-system
      # seconds
      thresholdRange:
         max: 0.5
      interval: 30s
    # testing (optional)
    webhooks:
      - name: smoke-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'anon' -H 'Host: www.example.com' http://keda-http-add-on-interceptor-proxy.keda:8080/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host www.example.com http://gateway-istio.istio-ingress/"
```

Save the above resource as `podinfo-canary.yaml` and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

After a couple of seconds Flagger will create the canary objects:

```bash
# applied
deployment.apps/podinfo
service/podinfo
httpscaledobject.http.keda.sh/podinfo
canary.flagger.app/podinfo

# generated
deployment.apps/podinfo-primary
service/podinfo-primary
httpscaledobject.http.keda.sh/podinfo-primary
httproutes.gateway.networking.k8s.io/podinfo
```

We refer to our HTTPScaledObject for the canary deployment using `.spec.autoscalerRef`. Flagger will use this to generate a primary HTTPScaledObject which will automatically scale their primary deployment up and down (including to/from zero) based on incoming HTTP traffic.

In the situation when it is desired to have different scaling replica configuration between the canary and primary deployment HTTPScaledObject you can use
the `.spec.autoscalerRef.primaryScalerReplicas` to override these values for the generated primary HTTPScaledObject.

After the boostrap, the podinfo deployment will be scaled to zero and the traffic to `podinfo.test` will be routed to the primary pods.
When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary. The Blue/Green deployment will run for five iterations while validating the HTTP metrics and rollout hooks every 15 seconds.

## Expose the app outside the cluster

Find the external address of Istio's load balancer:

```bash
export ADDRESS="$(kubectl -n istio-ingress get svc/gateway-istio -ojson \
| jq -r ".status.loadBalancer.ingress[].hostname")"
echo $ADDRESS
```

Configure your DNS server with a CNAME record \(AWS\) or A record \(GKE/AKS/DOKS\) and point a domain e.g. `www.example.com` to the LB address.

Now you can access the podinfo UI using your domain address.

Note that you should be using HTTPS when exposing production workloads on internet. You can obtain free TLS certs from Let's Encrypt, read this
[guide](https://github.com/stefanprodan/istio-gke) on how to configure cert-manager to secure Istio with TLS certificates.

If you're using a local cluster via kind/k3s you can port forward the Envoy LoadBalancer service:

```bash
kubectl port-forward -n istio-ingress svc/gateway-istio 8080:80
```

Now you can access podinfo via `curl -H "Host: www.example.com" localhost:8080`

## Automated canary promotion

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=stefanprodan/podinfo:6.0.1
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:         0
  Failed Checks:         0
  Phase:                 Succeeded
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    flagger  New revision detected podinfo.test
  Normal   Synced  3m    flagger  Scaling up podinfo.test
  Warning  Synced  3m    flagger  Waiting for podinfo.test rollout to finish: 0 of 1 updated replicas are available
  Normal   Synced  3m    flagger  Advance podinfo.test canary weight 5
  Normal   Synced  3m    flagger  Advance podinfo.test canary weight 10
  Normal   Synced  3m    flagger  Advance podinfo.test canary weight 15
  Normal   Synced  2m    flagger  Advance podinfo.test canary weight 20
  Normal   Synced  2m    flagger  Advance podinfo.test canary weight 25
  Normal   Synced  1m    flagger  Advance podinfo.test canary weight 30
  Normal   Synced  1m    flagger  Advance podinfo.test canary weight 35
  Normal   Synced  55s   flagger  Advance podinfo.test canary weight 40
  Normal   Synced  45s   flagger  Advance podinfo.test canary weight 45
  Normal   Synced  35s   flagger  Advance podinfo.test canary weight 50
  Normal   Synced  25s   flagger  Copying podinfo.test template spec to podinfo-primary.test
  Warning  Synced  15s   flagger  Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
  Normal   Synced  5s    flagger  Promotion completed! Scaling down podinfo.test
```

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

A canary deployment is triggered by changes in any of the following objects:

* Deployment PodSpec \(container image, command, ports, env, resources, etc\)
* ConfigMaps mounted as volumes or mapped to environment variables
* Secrets mounted as volumes or mapped to environment variables

You can monitor how Flagger progressively changes the weights of the HTTPRoute object that is attahed to the Gateway with:

```bash
watch kubectl get httproute -n test podinfo -o=jsonpath='{.spec.rules}'
```

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   15       2022-01-16T14:05:07Z
prod        frontend  Succeeded     0        2022-01-15T16:15:07Z
prod        backend   Failed        0        2022-01-14T17:05:07Z
```

## Automated rollback

During the canary analysis you can generate HTTP 500 errors and high latency to test if Flagger pauses the rollout.

Trigger another canary deployment:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=stefanprodan/podinfo:6.0.2
```

Exec into the load tester pod with:

```bash
kubectl -n test exec -it flagger-loadtester-xx-xx sh
```

Generate HTTP 500 errors:

```bash
watch curl -H 'Host: www.example.com' http://keda-http-add-on-interceptor-proxy.keda:8080/status/500
```

Generate latency:

```bash
watch curl -H 'Host: www.example.com' http://keda-http-add-on-interceptor-proxy.keda:8080/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary, the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:         0
  Failed Checks:         10
  Phase:                 Failed
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    flagger  Starting canary deployment for podinfo.test
  Normal   Synced  3m    flagger  Advance podinfo.test canary weight 5
  Normal   Synced  3m    flagger  Advance podinfo.test canary weight 10
  Normal   Synced  3m    flagger  Advance podinfo.test canary weight 15
  Normal   Synced  3m    flagger  Halt podinfo.test advancement error rate 69.17% > 1%
  Normal   Synced  2m    flagger  Halt podinfo.test advancement error rate 61.39% > 1%
  Normal   Synced  2m    flagger  Halt podinfo.test advancement error rate 55.06% > 1%
  Normal   Synced  2m    flagger  Halt podinfo.test advancement error rate 47.00% > 1%
  Normal   Synced  2m    flagger  (combined from similar events): Halt podinfo.test advancement error rate 38.08% > 1%
  Warning  Synced  1m    flagger  Rolling back podinfo.test failed checks threshold reached 10
  Warning  Synced  1m    flagger  Canary failed! Scaling down podinfo.test
```

## Session Affinity

While Flagger can perform weighted routing and A/B testing individually, with Gateway API it can combine the two leading to a Canary
release with session affinity.
For more information you can read the [deployment strategies docs](../usage/deployment-strategies.md#canary-release-with-session-affinity).

> **Note:** The implementation must have support for the [`ResponseHeaderModifier`](https://github.com/kubernetes-sigs/gateway-api/blob/3d22aa5a08413222cb79e6b2e245870360434614/apis/v1beta1/httproute_types.go#L651) API.

Create a canary custom resource \(replace <www.example.com> with your own domain\):

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
  # HTTPScaledObject reference
  autoscalerRef:
    apiVersion: keda.sh/v1alpha1
    kind: HTTPScaledObject
    name: podinfo
    primaryScalingSet:
      name: primary
      kind: ClusterHTTPScalingSet
  service:
    # service port number
    port: 9898
    # Reference to the Gateway that the generated HTTPRoute would attach to.
    gatewayRefs:
      - name: gateway
        namespace: istio-ingress
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
    # session affinity config
    sessionAffinity:
      # name of the cookie used
      cookieName: flagger-cookie
      # max age of the cookie (in seconds)
      # optional; defaults to 86400
      maxAge: 21600
    metrics:
    - name: error-rate
      # max error rate (5xx responses)
      # percentage (0-100)
      templateRef:
        name: error-rate
        namespace: flagger-system
      thresholdRange:
        max: 1
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: flagger-system
      # seconds
      thresholdRange:
         max: 0.5
      interval: 30s
    # testing (optional)
    webhooks:
      - name: smoke-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'anon' -H 'Host: www.example.com' http://keda-http-add-on-interceptor-proxy.keda:8080/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host www.example.com http://gateway-istio.istio-ingress/"
```

Save the above resource as podinfo-canary-session-affinity.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary-session-affinity.yaml
```

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.1
```

You can load `www.example.com` in your browser and refresh it until you see the requests being served by `podinfo:6.0.1`.
All subsequent requests after that will be served by `podinfo:6.0.1` and not `podinfo:6.0.0` because of the session affinity
configured by Flagger in the HTTPRoute object.

# A/B Testing

Besides weighted routing, Flagger can be configured to route traffic to the canary based on HTTP match conditions. In an A/B testing scenario, you'll be using HTTP headers or cookies to target a certain segment of your users. This is particularly useful for frontend applications that require session affinity.

![Flagger A/B Testing Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-abtest-steps.png)

Create a canary custom resource \(replace "www.example.com" with your own domain\):

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
  # HTTPScaledObject reference
  autoscalerRef:
    apiVersion: keda.sh/v1alpha1
    kind: HTTPScaledObject
    name: podinfo
    primaryScalingSet:
      name: primary
      kind: ClusterHTTPScalingSet
  service:
    # service port number
    port: 9898
    # Reference to the Gateway that the generated HTTPRoute would attach to.
    gatewayRefs:
      - name: gateway
        namespace: istio-ingress
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
    - name: error-rate
      # max error rate (5xx responses)
      # percentage (0-100)
      templateRef:
        name: error-rate
        namespace: flagger-system
      thresholdRange:
        max: 1
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: flagger-system
      # seconds
      thresholdRange:
         max: 0.5
      interval: 30s
    # testing (optional)
    webhooks:
      - name: smoke-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'anon' -H 'Host: www.example.com' http://keda-http-add-on-interceptor-proxy.keda:8080/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host www.example.com -H 'X-Canary: insider' http://gateway-istio.istio-ingress/"
```

The above configuration will run an analysis for ten minutes targeting those users that have an insider cookie.

Save the above resource as podinfo-ab-canary.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-ab-canary.yaml
```

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=stefanprodan/podinfo:6.0.3
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/podinfo

Status:
  Failed Checks:         0
  Phase:                 Succeeded
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    flagger  New revision detected podinfo.test
  Normal   Synced  3m    flagger  Scaling up podinfo.test
  Warning  Synced  3m    flagger  Waiting for podinfo.test rollout to finish: 0 of 1 updated replicas are available
  Normal   Synced  3m    flagger  Advance podinfo.test canary iteration 1/10
  Normal   Synced  3m    flagger  Advance podinfo.test canary iteration 2/10
  Normal   Synced  3m    flagger  Advance podinfo.test canary iteration 3/10
  Normal   Synced  2m    flagger  Advance podinfo.test canary iteration 4/10
  Normal   Synced  2m    flagger  Advance podinfo.test canary iteration 5/10
  Normal   Synced  1m    flagger  Advance podinfo.test canary iteration 6/10
  Normal   Synced  1m    flagger  Advance podinfo.test canary iteration 7/10
  Normal   Synced  55s   flagger  Advance podinfo.test canary iteration 8/10
  Normal   Synced  45s   flagger  Advance podinfo.test canary iteration 9/10
  Normal   Synced  35s   flagger  Advance podinfo.test canary iteration 10/10
  Normal   Synced  25s   flagger  Copying podinfo.test template spec to podinfo-primary.test
  Warning  Synced  15s   flagger  Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
  Normal   Synced  5s    flagger  Promotion completed! Scaling down podinfo.test
```

## Traffic mirroring

![Flagger Canary Traffic Shadowing](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-canary-traffic-mirroring.png)

For applications that perform read operations, Flagger can be configured to do B/G tests with traffic mirroring.
Gateway API traffic mirroring will copy each incoming request, sending one request to the primary and one to the canary service.
The response from the primary is sent back to the user and the response from the canary is discarded.
Metrics are collected on both requests so that the deployment will only proceed if the canary metrics are within the threshold values.

Note that mirroring should be used for requests that are **idempotent** or capable of being processed twice \(once by the primary and once by the canary\).

You can enable mirroring by replacing `stepWeight` with `iterations` and by setting `analysis.mirror` to `true`:

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
  service:
    # service port number
    port: 9898
    # container port number or name (optional)
    targetPort: 9898
    # Gateway API HTTPRoute host names
    hosts:
     - www.example.com
    # Reference to the Gateway that the generated HTTPRoute would attach to.
    gatewayRefs:
      - name: gateway
        namespace: istio-ingress
  analysis:
    # schedule interval
    interval: 1m
    # max number of failed metric checks before rollback
    threshold: 5
    # total number of iterations
    iterations: 10
    # enable traffic shadowing
    mirror: true
    # Gateway API HTTPRoute host names
    metrics:
      - name: request-success-rate
        thresholdRange:
          min: 99
        interval: 1m
      - name: request-duration
        thresholdRange:
          max: 500
        interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host www.example.com http://gateway-istio.istio-ingress/"
```

With the above configuration, Flagger will run a canary release with the following steps:

* detect new revision \(deployment spec, secrets or configmaps changes\)
* scale from zero the canary deployment
* wait for the HPA to set the canary minimum replicas
* check canary pods health
* run the acceptance tests
* abort the canary release if tests fail
* start the load tests
* mirror 100% of the traffic from primary to canary
* check request success rate and request duration every minute
* abort the canary release if the metrics check failure threshold is reached
* stop traffic mirroring after the number of iterations is reached
* route live traffic to the canary pods
* promote the canary \(update the primary secrets, configmaps and deployment spec\)
* wait for the primary deployment rollout to finish
* wait for the HPA to set the primary minimum replicas
* check primary pods health
* switch live traffic back to primary
* scale to zero the canary
* send notification with the canary analysis result

The above procedures can be extended with [custom metrics](../usage/metrics.md) checks, [webhooks](../usage/webhooks.md), [manual promotion](../usage/webhooks.md#manual-gating) approval and [Slack or MS Teams](../usage/alerting.md) notifications.
