# Linkerd Canary Deployments

This guide shows you how to use Linkerd and Flagger to automate canary deployments.

## Prerequisites

Flagger requires a Kubernetes cluster **v1.21** or newer and Linkerd **2.14** or newer.

Install Linkerd and Prometheus (part of Linkerd Viz):

```bash
# The CRDs need to be installed beforehand
linkerd install --crds | kubectl apply -f -

linkerd install | kubectl apply -f -
linkerd viz install | kubectl apply -f -

# For linkerd versions 2.12 and later, the SMI extension needs to be install in
# order to enable TrafficSplits
curl -sL https://linkerd.github.io/linkerd-smi/install | sh
linkerd smi install | kubectl apply -f -
```

Install Flagger in the flagger-system namespace:

```bash
kubectl apply -k github.com/fluxcd/flagger//kustomize/linkerd
```

If you prefer Helm, these are the commands to install Linkerd, Linkerd Viz,
Linkerd-SMI and Flagger:

```bash
helm repo add linkerd https://helm.linkerd.io/stable
helm install linkerd-crds linkerd/linkerd-crds -n linkerd --create-namespace
# See https://linkerd.io/2/tasks/generate-certificates/ for how to generate the
# certs referred below
helm install linkerd-control-plane linkerd/linkerd-control-plane \
  -n linkerd \
  --set-file identityTrustAnchorsPEM=ca.crt \
  --set-file identity.issuer.tls.crtPEM=issuer.crt \
  --set-file identity.issuer.tls.keyPEM=issuer.key \

helm install linkerd-viz linkerd/linkerd-viz -n linkerd-viz --create-namespace

helm install flagger flagger/flagger \
  --n flagger-system \
  --set meshProvider=gatewayapi:v1beta1 \
  --set metricsServer=http://prometheus.linkerd-viz:9090 \
  --set linkerdAuthPolicy.create=true
```

## Bootstrap

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services and SMI traffic split).
These objects expose the application inside the mesh and drive the canary analysis and promotion.

Create a test namespace and enable Linkerd proxy injection:

```bash
kubectl create ns test
kubectl annotate namespace test linkerd.io/inject=enabled
```

Install the load testing service to generate traffic during the canary analysis:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/tester?ref=main
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/podinfo?ref=main
```

Create a canary custom resource for the podinfo deployment:

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
  # HPA reference (optional)
  autoscalerRef:
    apiVersion: autoscaling/v2
    kind: HorizontalPodAutoscaler
    name: podinfo
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  service:
    # ClusterIP port number
    port: 9898
    # container port number or name (optional)
    targetPort: 9898
    # Reference to the Service that the generated HTTPRoute would attach to.
    gatewayRefs:
      - name: podinfo
        namespace: test
        group: core
        kind: Service
        port: 9898
  analysis:
    # schedule interval (default 60s)
    interval: 30s
    # max number of failed metric checks before rollback
    threshold: 5
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 5
    # Linkerd Prometheus checks
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
          cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 http://podinfo-canary.test:9898/"
```

Save the above resource as podinfo-canary.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary. The canary analysis will run for five minutes while validating the HTTP metrics and rollout hooks every half a minute.

After a couple of seconds Flagger will create the canary objects:

```bash
# applied
deployment.apps/podinfo
horizontalpodautoscaler.autoscaling/podinfo
ingresses.extensions/podinfo
canary.flagger.app/podinfo

# generated
deployment.apps/podinfo-primary
horizontalpodautoscaler.autoscaling/podinfo-primary
service/podinfo
service/podinfo-canary
service/podinfo-primary
trafficsplits.split.smi-spec.io/podinfo
```

After the bootstrap, the podinfo deployment will be scaled to zero and the traffic to `podinfo.test` will be routed to the primary pods. During the canary analysis, the `podinfo-canary.test` address can be used to target directly the canary pods.

## Automated canary promotion

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance indicators like HTTP requests success rate, requests average duration and pod health. Based on analysis of the KPIs a canary is promoted or aborted, and the analysis result is published to Slack.

![Flagger Canary Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-canary-steps.png)

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.1
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:         0
  Failed Checks:         0
  Phase:                 Succeeded
Events:
 New revision detected! Scaling up podinfo.test
 Waiting for podinfo.test rollout to finish: 0 of 1 updated replicas are available
 Pre-rollout check acceptance-test passed
 Advance podinfo.test canary weight 5
 Advance podinfo.test canary weight 10
 Advance podinfo.test canary weight 15
 Advance podinfo.test canary weight 20
 Advance podinfo.test canary weight 25
 Waiting for podinfo.test rollout to finish: 1 of 2 updated replicas are available
 Advance podinfo.test canary weight 30
 Advance podinfo.test canary weight 35
 Advance podinfo.test canary weight 40
 Advance podinfo.test canary weight 45
 Advance podinfo.test canary weight 50
 Copying podinfo.test template spec to podinfo-primary.test
 Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
 Promotion completed! Scaling down podinfo.test
```

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

A canary deployment is triggered by changes in any of the following objects:

* Deployment PodSpec \(container image, command, ports, env, resources, etc\)
* ConfigMaps mounted as volumes or mapped to environment variables
* Secrets mounted as volumes or mapped to environment variables

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   15       2019-06-30T14:05:07Z
prod        frontend  Succeeded     0        2019-06-30T16:15:07Z
prod        backend   Failed        0        2019-06-30T17:05:07Z
```

## Automated rollback

During the canary analysis you can generate HTTP 500 errors and high latency to test if Flagger pauses and rolls back the faulted version.

Trigger another canary deployment:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.2
```

Exec into the load tester pod with:

```bash
kubectl -n test exec -it flagger-loadtester-xx-xx sh
```

Generate HTTP 500 errors:

```bash
watch -n 1 curl http://podinfo-canary.test:9898/status/500
```

Generate latency:

```bash
watch -n 1 curl http://podinfo-canary.test:9898/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary, the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n test describe canary/podinfo

Status:
  Canary Weight:         0
  Failed Checks:         10
  Phase:                 Failed
Events:
 Starting canary analysis for podinfo.test
 Pre-rollout check acceptance-test passed
 Advance podinfo.test canary weight 5
 Advance podinfo.test canary weight 10
 Advance podinfo.test canary weight 15
 Halt podinfo.test advancement success rate 69.17% < 99%
 Halt podinfo.test advancement success rate 61.39% < 99%
 Halt podinfo.test advancement success rate 55.06% < 99%
 Halt podinfo.test advancement request duration 1.20s > 0.5s
 Halt podinfo.test advancement request duration 1.45s > 0.5s
 Rolling back podinfo.test failed checks threshold reached 5
 Canary failed! Scaling down podinfo.test
```

## Custom metrics

The canary analysis can be extended with Prometheus queries.

Let's a define a check for not found errors. Edit the canary analysis and add the following metric:

```yaml
  analysis:
    metrics:
    - name: "404s percentage"
      threshold: 3
      query: |
        100 - sum(
            rate(
                response_total{
                    namespace="test",
                    deployment="podinfo",
                    status_code!="404",
                    direction="inbound"
                }[1m]
            )
        )
        /
        sum(
            rate(
                response_total{
                    namespace="test",
                    deployment="podinfo",
                    direction="inbound"
                }[1m]
            )
        )
        * 100
```

The above configuration validates the canary version by checking if the HTTP 404 req/sec percentage is below three percent of the total traffic. If the 404s rate reaches the 3% threshold, then the analysis is aborted and the canary is marked as failed.

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.3
```

Generate 404s:

```bash
watch -n 1 curl http://podinfo-canary:9898/status/404
```

Watch Flagger logs:

```text
kubectl -n flagger-system logs deployment/flagger -f | jq .msg

Starting canary deployment for podinfo.test
Pre-rollout check acceptance-test passed
Advance podinfo.test canary weight 5
Halt podinfo.test advancement 404s percentage 6.20 > 3
Halt podinfo.test advancement 404s percentage 6.45 > 3
Halt podinfo.test advancement 404s percentage 7.22 > 3
Halt podinfo.test advancement 404s percentage 6.50 > 3
Halt podinfo.test advancement 404s percentage 6.34 > 3
Rolling back podinfo.test failed checks threshold reached 5
Canary failed! Scaling down podinfo.test
```

If you have Slack configured, Flagger will send a notification with the reason why the canary failed.

## Linkerd Ingress

There are two ingress controllers that are compatible with both Flagger and Linkerd: NGINX and Gloo.

Install NGINX:

```bash
helm upgrade -i nginx-ingress stable/nginx-ingress \
--namespace ingress-nginx
```

Create an ingress definition for podinfo that rewrites the incoming header to the internal service name \(required by Linkerd\):

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: podinfo
  namespace: test
  labels:
    app: podinfo
  annotations:
    kubernetes.io/ingress.class: "nginx"
    nginx.ingress.kubernetes.io/configuration-snippet: |
      proxy_set_header l5d-dst-override $service_name.$namespace.svc.cluster.local:9898;
      proxy_hide_header l5d-remote-ip;
      proxy_hide_header l5d-server-id;
spec:
  rules:
    - host: app.example.com
      http:
        paths:
          - backend:
              serviceName: podinfo
              servicePort: 9898
```

When using an ingress controller, the Linkerd traffic split does not apply to incoming traffic since NGINX in running outside of the mesh. In order to run a canary analysis for a frontend app, Flagger creates a shadow ingress and sets the NGINX specific annotations.

## A/B Testing

Besides weighted routing, Flagger can be configured to route traffic to the canary based on HTTP match conditions. In an A/B testing scenario, you'll be using HTTP headers or cookies to target a certain segment of your users. This is particularly useful for frontend applications that require session affinity.

![Flagger Linkerd Ingress](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-nginx-linkerd.png)

Edit podinfo canary analysis, set the provider to `nginx`, add the ingress reference, remove the max/step weight and add the match conditions and iterations:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  # ingress reference
  provider: nginx
  ingressRef:
    apiVersion: extensions/v1beta1
    kind: Ingress
    name: podinfo
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  autoscalerRef:
    apiVersion: autoscaling/v2
    kind: HorizontalPodAutoscaler
    name: podinfo
  service:
    # container port
    port: 9898
  analysis:
    interval: 1m
    threshold: 10
    iterations: 10
    match:
      # curl -H 'X-Canary: always' http://app.example.com
      - headers:
          x-canary:
            exact: "always"
      # curl -b 'canary=always' http://app.example.com
      - headers:
          cookie:
            exact: "canary"
    # Linkerd Prometheus checks
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      thresholdRange:
        max: 500
      interval: 30s
    webhooks:
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -H 'Cookie: canary=always' http://app.example.com"
```

The above configuration will run an analysis for ten minutes targeting users that have a `canary` cookie set to `always` or those that call the service using the `X-Canary: always` header.

**Note** that the load test now targets the external address and uses the canary cookie.

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.4
```

Flagger detects that the deployment revision changed and starts the A/B testing:

```text
kubectl -n test describe canary/podinfo

Events:
 Starting canary deployment for podinfo.test
 Pre-rollout check acceptance-test passed
 Advance podinfo.test canary iteration 1/10
 Advance podinfo.test canary iteration 2/10
 Advance podinfo.test canary iteration 3/10
 Advance podinfo.test canary iteration 4/10
 Advance podinfo.test canary iteration 5/10
 Advance podinfo.test canary iteration 6/10
 Advance podinfo.test canary iteration 7/10
 Advance podinfo.test canary iteration 8/10
 Advance podinfo.test canary iteration 9/10
 Advance podinfo.test canary iteration 10/10
 Copying podinfo.test template spec to podinfo-primary.test
 Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
 Promotion completed! Scaling down podinfo.test
```

The above procedure can be extended with [custom metrics](../usage/metrics.md) checks, [webhooks](../usage/webhooks.md), [manual promotion](../usage/webhooks.md#manual-gating) approval and [Slack or MS Teams](../usage/alerting.md) notifications.

