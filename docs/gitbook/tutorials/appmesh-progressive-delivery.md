# App Mesh Canary Deployments

This guide shows you how to use App Mesh and Flagger to automate canary deployments.
You'll need an EKS cluster (Kubernetes >= 1.16) configured with App Mesh,
you can find the installation guide [here](https://docs.flagger.app/install/flagger-install-on-eks-appmesh).

## Bootstrap

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler (HPA),
then creates a series of objects (Kubernetes deployments, ClusterIP services,
App Mesh virtual nodes and services).
These objects expose the application on the mesh and drive the canary analysis and promotion.
The only App Mesh object you need to create by yourself is the mesh resource.

Create a mesh called `global`:

```bash
cat << EOF | kubectl apply -f -
apiVersion: appmesh.k8s.aws/v1beta2
kind: Mesh
metadata:
  name: global
spec:
  namespaceSelector:
    matchLabels:
      appmesh.k8s.aws/sidecarInjectorWebhook: enabled
EOF
```

Create a test namespace with App Mesh sidecar injection enabled:

```bash
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: test
  labels:
    appmesh.k8s.aws/sidecarInjectorWebhook: enabled
EOF
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/podinfo?ref=main
```

Deploy the load testing service to generate traffic during the canary analysis:

```bash
helm upgrade -i flagger-loadtester flagger/loadtester \
--namespace=test \
--set appmesh.enabled=true \
--set "appmesh.backends[0]=podinfo" \
--set "appmesh.backends[1]=podinfo-canary"
```

Create a canary definition:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  annotations:
    # Enable Envoy access logging to stdout.
    appmesh.flagger.app/accesslog: enabled
  name: podinfo
  namespace: test
spec:
  # App Mesh API reference
  provider: appmesh:v1beta2
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
    apiVersion: autoscaling/v2
    kind: HorizontalPodAutoscaler
    name: podinfo
  service:
    # container port
    port: 9898
    # App Mesh ingress timeout (optional)
    timeout: 15s
    # App Mesh retry policy (optional)
    retries:
      attempts: 3
      perTryTimeout: 5s
      retryOn: "gateway-error,client-error,stream-error"
    # App Mesh URI settings
    match:
      - uri:
          prefix: /
    rewrite:
      uri: /
  # define the canary analysis timing and KPIs
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
    stepWeight: 5
    # App Mesh Prometheus checks
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
      url: http://flagger-loadtester.test/
      timeout: 5s
      metadata:
        cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test:9898/"
```

Save the above resource as podinfo-canary.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

After a couple of seconds Flagger will create the canary objects:

```bash
# applied 
deployment.apps/podinfo
horizontalpodautoscaler.autoscaling/podinfo
canary.flagger.app/podinfo

# generated Kubernetes objects
deployment.apps/podinfo-primary
horizontalpodautoscaler.autoscaling/podinfo-primary
service/podinfo
service/podinfo-canary
service/podinfo-primary

# generated App Mesh objects
virtualnode.appmesh.k8s.aws/podinfo-canary
virtualnode.appmesh.k8s.aws/podinfo-primary
virtualrouter.appmesh.k8s.aws/podinfo
virtualrouter.appmesh.k8s.aws/podinfo-canary
virtualservice.appmesh.k8s.aws/podinfo
virtualservice.appmesh.k8s.aws/podinfo-canary
```

After the bootstrap, the podinfo deployment will be scaled to zero and the traffic to `podinfo.test`
will be routed to the primary pods.
During the canary analysis, the `podinfo-canary.test` address can be used to target directly the canary pods.

App Mesh blocks all egress traffic by default.
If your application needs to call another service, you have to create an App Mesh virtual service for it
and add the virtual service name to the backend list.

```yaml
  service:
    port: 9898
    backends:
      - backend1
      - arn:aws:appmesh:eu-west-1:12345678910:mesh/my-mesh/virtualService/backend2
```

## Setup App Mesh Gateway (optional)

In order to expose the podinfo app outside the mesh you can use the App Mesh Gateway.

Deploy the App Mesh Gateway behind an AWS NLB:

```bash
helm upgrade -i appmesh-gateway eks/appmesh-gateway \
--namespace test
```

Find the gateway public address:

```bash
export URL="http://$(kubectl -n test get svc/appmesh-gateway -ojson | jq -r ".status.loadBalancer.ingress[].hostname")"
echo $URL
```

Wait for the NLB to become active:

```bash
 watch curl -sS $URL
```

Create a gateway route that points to the podinfo virtual service:

```yaml
cat << EOF | kubectl apply -f -
apiVersion: appmesh.k8s.aws/v1beta2
kind: GatewayRoute
metadata:
  name: podinfo
  namespace: test
spec:
  httpRoute:
    match:
      prefix: "/"
    action:
      target:
        virtualService:
          virtualServiceRef:
            name: podinfo
EOF
```

Open your browser and navigate to the ingress address to access podinfo UI.

## Automated canary promotion

A canary deployment is triggered by changes in any of the following objects:

* Deployment PodSpec (container image, command, ports, env, resources, etc)
* ConfigMaps and Secrets mounted as volumes or mapped to environment variables

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
 Advance podinfo.test canary weight 30
 Advance podinfo.test canary weight 35
 Advance podinfo.test canary weight 40
 Advance podinfo.test canary weight 45
 Advance podinfo.test canary weight 50
 Copying podinfo.test template spec to podinfo-primary.test
 Waiting for podinfo-primary.test rollout to finish: 1 of 2 updated replicas are available
 Routing all traffic to primary
 Promotion completed! Scaling down podinfo.test
```

When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary.

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

During the analysis the canary’s progress can be monitored with Grafana.
The App Mesh dashboard URL is
[http://localhost:3000/d/flagger-appmesh/appmesh-canary?refresh=10s&orgId=1&var-namespace=test&var-primary=podinfo-primary&var-canary=podinfo](http://localhost:3000/d/flagger-appmesh/appmesh-canary?refresh=10s&orgId=1&var-namespace=test&var-primary=podinfo-primary&var-canary=podinfo).

![App Mesh Canary Dashboard](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/screens/flagger-grafana-appmesh.png)

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT
test        podinfo   Progressing   15
prod        frontend  Succeeded     0
prod        backend   Failed        0
```

If you’ve enabled the Slack notifications, you should receive the following messages:

![Flagger Slack Notifications](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/screens/slack-canary-notifications.png)

## Automated rollback

During the canary analysis you can generate HTTP 500 errors or high latency to test if Flagger pauses the rollout.

Trigger a canary deployment:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.2
```

Exec into the load tester pod with:

```bash
kubectl -n test exec -it deploy/flagger-loadtester bash
```

Generate HTTP 500 errors:

```bash
hey -z 1m -c 5 -q 5 http://podinfo-canary.test:9898/status/500
```

Generate latency:

```bash
watch -n 1 curl http://podinfo-canary.test:9898/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary,
the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n appmesh-system logs deploy/flagger -f | jq .msg

New revision detected! progressing canary analysis for podinfo.test
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

If you’ve enabled the Slack notifications, you’ll receive a message if the progress deadline is exceeded,
or if the analysis reached the maximum number of failed checks:

![Flagger Slack Notifications](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/screens/slack-canary-failed.png)

## A/B Testing

Besides weighted routing, Flagger can be configured to route traffic to the canary based on HTTP match conditions.
In an A/B testing scenario, you'll be using HTTP headers or cookies to target a certain segment of your users.
This is particularly useful for frontend applications that require session affinity.

![Flagger A/B Testing Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-abtest-steps.png)

Edit the canary analysis, remove the max/step weight and add the match conditions and iterations:

```yaml
  analysis:
    interval: 1m
    threshold: 5
    iterations: 10
    match:
    - headers:
        x-canary:
          exact: "insider"
    webhooks:
    - name: load-test
      url: http://flagger-loadtester.test/
      metadata:
        cmd: "hey -z 1m -q 10 -c 2 -H 'X-Canary: insider' http://podinfo.test:9898/"
```

The above configuration will run an analysis for ten minutes targeting users that have a `X-Canary: insider` header.

You can also use a HTTP cookie, to target all users with a `canary` cookie set to `insider` the match condition should be:

```yaml
match:
- headers:
    cookie:
      regex: "^(.*?;)?(canary=insider)(;.*)?$"
webhooks:
- name: load-test
  url: http://flagger-loadtester.test/
  metadata:
    cmd: "hey -z 1m -q 10 -c 2 -H 'Cookie: canary=insider' http://podinfo.test:9898/"
```

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.3
```

Flagger detects that the deployment revision changed and starts the A/B test:

```text
kubectl -n appmesh-system logs deploy/flagger -f | jq .msg

New revision detected! progressing canary analysis for podinfo.test
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
Routing all traffic to primary
Promotion completed! Scaling down podinfo.test
```

The above procedure can be extended with
[custom metrics](../usage/metrics.md) checks,
[webhooks](../usage/webhooks.md),
[manual promotion](../usage/webhooks.md#manual-gating) approval and
[Slack or MS Teams](../usage/alerting.md) notifications.
