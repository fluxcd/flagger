# Contour Canary Deployments

This guide shows you how to use [Contour](https://projectcontour.io/) ingress controller and Flagger to automate canary releases and A/B testing.

![Flagger Contour Overview](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-contour-overview.png)

## Prerequisites

Flagger requires a Kubernetes cluster **v1.16** or newer and Contour **v1.0** or newer.

Install Contour on a cluster with LoadBalancer support:

```bash
kubectl apply -f https://projectcontour.io/quickstart/contour.yaml
```

The above command will deploy Contour and an Envoy daemonset in the `projectcontour` namespace.

Install Flagger using Kustomize (kubectl 1.14) in the `projectcontour` namespace:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/contour?ref=main
```

The above command will deploy Flagger and Prometheus configured to scrape the Contour's Envoy instances.

Or you can install Flagger using Helm v3:

```bash
helm repo add flagger https://flagger.app

helm upgrade -i flagger flagger/flagger \
--namespace projectcontour \
--set meshProvider=contour \
--set ingressClass=contour \
--set prometheus.install=true
```

You can also enable Slack, Discord, Rocket or MS Teams notifications, see the alerting [docs](../usage/alerting.md).

## Bootstrap

Flagger takes a Kubernetes deployment and optionally a horizontal pod autoscaler \(HPA\), then creates a series of objects \(Kubernetes deployments, ClusterIP services and Contour HTTPProxy\). These objects expose the application in the cluster and drive the canary analysis and promotion.

Create a test namespace:

```bash
kubectl create ns test
```

Install the load testing service to generate traffic during the canary analysis:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/tester?ref=main
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/podinfo?ref=main
```

Create a canary custom resource \(replace `app.example.com` with your own domain\):

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
  # HPA reference
  autoscalerRef:
    apiVersion: autoscaling/v2
    kind: HorizontalPodAutoscaler
    name: podinfo
  service:
    # service port
    port: 80
    # container port
    targetPort: 9898
    # Contour request timeout
    timeout: 15s
    # Contour retry policy
    retries:
      attempts: 3
      perTryTimeout: 5s
      # supported values for retryOn - https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.RetryOn
      retryOn: "5xx"
  # define the canary analysis timing and KPIs
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
    # Contour Prometheus checks
    metrics:
    - name: request-success-rate
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      # maximum req duration P99 in milliseconds
      thresholdRange:
        max: 500
      interval: 30s
    # testing
    webhooks:
    - name: acceptance-test
      type: pre-rollout
      url: http://flagger-loadtester.test/
      timeout: 30s
      metadata:
        type: bash
        cmd: "curl -sd 'test' http://podinfo-canary.test/token | grep token"
    - name: load-test
      url: http://flagger-loadtester.test/
      type: rollout
      timeout: 5s
      metadata:
        cmd: "hey -z 1m -q 10 -c 2 -host app.example.com http://envoy.projectcontour"
```

Save the above resource as podinfo-canary.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

The canary analysis will run for five minutes while validating the HTTP metrics and rollout hooks every half a minute.

After a couple of seconds Flagger will create the canary objects:

```bash
# applied 
deployment.apps/podinfo
horizontalpodautoscaler.autoscaling/podinfo
canary.flagger.app/podinfo

# generated
deployment.apps/podinfo-primary
horizontalpodautoscaler.autoscaling/podinfo-primary
service/podinfo
service/podinfo-canary
service/podinfo-primary
httpproxy.projectcontour.io/podinfo
```

After the bootstrap, the podinfo deployment will be scaled to zero and the traffic to `podinfo.test` will be routed to the primary pods. During the canary analysis, the `podinfo-canary.test` address can be used to target directly the canary pods.

## Expose the app outside the cluster

Find the external address of Contour's Envoy load balancer:

```bash
export ADDRESS="$(kubectl -n projectcontour get svc/envoy -ojson \
| jq -r ".status.loadBalancer.ingress[].hostname")"
echo $ADDRESS
```

Configure your DNS server with a CNAME record \(AWS\) or A record \(GKE/AKS/DOKS\) and point a domain e.g. `app.example.com` to the LB address.

Create a HTTPProxy definition and include the podinfo proxy generated by Flagger \(replace `app.example.com` with your own domain\):

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: podinfo-ingress
  namespace: test
spec:
  virtualhost:
    fqdn: app.example.com
  includes:
    - name: podinfo
      namespace: test
      conditions:
        - prefix: /
```

Save the above resource as podinfo-ingress.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-ingress.yaml
```

Verify that Contour processed the proxy definition with:

```bash
kubectl -n test get httpproxies

NAME              FQDN                STATUS
podinfo                               valid
podinfo-ingress   app.example.com     valid
```

Now you can access podinfo UI using your domain address.

Note that you should be using HTTPS when exposing production workloads on internet. You can obtain free TLS certs from Let's Encrypt, read this [guide](https://github.com/stefanprodan/eks-contour-ingress) on how to configure cert-manager to secure Contour with TLS certificates.

## Automated canary promotion

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance indicators like HTTP requests success rate, requests average duration and pod health. Based on analysis of the KPIs a canary is promoted or aborted.

![Flagger Canary Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-canary-steps.png)

A canary deployment is triggered by changes in any of the following objects:

* Deployment PodSpec \(container image, command, ports, env, resources, etc\)
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

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   15       2019-12-20T14:05:07Z
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
hey -z 1m -c 5 -q 5 http://app.example.com/status/500
```

Generate latency:

```bash
watch -n 1 curl http://app.example.com/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary, the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n projectcontour logs deploy/flagger -f | jq .msg

New revision detected! progressing canary analysis for podinfo.test
Pre-rollout check acceptance-test passed
Advance podinfo.test canary weight 5
Advance podinfo.test canary weight 10
Advance podinfo.test canary weight 15
Halt podinfo.test advancement success rate 69.17% < 99%
Halt podinfo.test advancement success rate 61.39% < 99%
Halt podinfo.test advancement success rate 55.06% < 99%
Halt podinfo.test advancement request duration 1.20s > 500ms
Halt podinfo.test advancement request duration 1.45s > 500ms
Rolling back podinfo.test failed checks threshold reached 5
Canary failed! Scaling down podinfo.test
```

If you’ve enabled the Slack notifications, you’ll receive a message if the progress deadline is exceeded, or if the analysis reached the maximum number of failed checks:

![Flagger Slack Notifications](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/screens/slack-canary-failed.png)

## A/B Testing

Besides weighted routing, Flagger can be configured to route traffic to the canary based on HTTP match conditions. In an A/B testing scenario, you'll be using HTTP headers or cookies to target a certain segment of your users. This is particularly useful for frontend applications that require session affinity.

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
      cmd: "hey -z 1m -q 5 -c 5 -H 'X-Canary: insider' -host app.example.com http://envoy.projectcontour"
```

The above configuration will run an analysis for ten minutes targeting users that have a `X-Canary: insider` header.

You can also use a HTTP cookie. To target all users with a cookie set to `insider`, the match condition should be:

```yaml
match:
- headers:
    cookie:
      suffix: "insider"
webhooks:
- name: load-test
  url: http://flagger-loadtester.test/
  metadata:
    cmd: "hey -z 1m -q 5 -c 5 -H 'Cookie: canary=insider' -host app.example.com http://envoy.projectcontour"
```

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/podinfo \
podinfod=ghcr.io/stefanprodan/podinfo:6.0.3
```

Flagger detects that the deployment revision changed and starts the A/B test:

```text
kubectl -n projectcontour logs deploy/flagger -f | jq .msg

New revision detected! Progressing canary analysis for podinfo.test
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

The web browser user agent header allows user segmentation based on device or OS.

For example, if you want to route all mobile users to the canary instance:

```yaml
match:
- headers:
    user-agent:
      prefix: "Mobile"
```

Or if you want to target only Android users:

```yaml
match:
- headers:
    user-agent:
      prefix: "Android"
```

Or a specific browser version:

```yaml
match:
- headers:
    user-agent:
      suffix: "Firefox/71.0"
```

For an in-depth look at the analysis process read the [usage docs](../usage/how-it-works.md).

