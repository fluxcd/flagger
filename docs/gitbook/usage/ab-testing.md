# Istio A/B Testing 

This guide shows you how to automate A/B testing with Istio and Flagger.

Besides weighted routing, Flagger can be configured to route traffic to the canary based on HTTP match conditions.
In an A/B testing scenario, you'll be using HTTP headers or cookies to target a certain segment of your users.
This is particularly useful for frontend applications that require session affinity.

![Flagger A/B Testing Stages](https://raw.githubusercontent.com/weaveworks/flagger/master/docs/diagrams/flagger-abtest-steps.png)

### Bootstrap

Create a test namespace with Istio sidecar injection enabled:

```bash
export REPO=https://raw.githubusercontent.com/weaveworks/flagger/master

kubectl apply -f ${REPO}/artifacts/namespaces/test.yaml
```

Create a deployment and a horizontal pod autoscaler:

```bash
kubectl apply -f ${REPO}/artifacts/ab-testing/deployment.yaml
kubectl apply -f ${REPO}/artifacts/ab-testing/hpa.yaml
```

Deploy the load testing service to generate traffic during the canary analysis:

```bash
kubectl -n test apply -f ${REPO}/artifacts/loadtester/deployment.yaml
kubectl -n test apply -f ${REPO}/artifacts/loadtester/service.yaml
```

Create a canary custom resource (replace example.com with your own domain):

```yaml
apiVersion: flagger.app/v1alpha3
kind: Canary
metadata:
  name: abtest
  namespace: test
spec:
  # deployment reference
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: abtest
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  # HPA reference (optional)
  autoscalerRef:
    apiVersion: autoscaling/v2beta1
    kind: HorizontalPodAutoscaler
    name: abtest
  service:
    # container port
    port: 9898
    # Istio gateways (optional)
    gateways:
    - public-gateway.istio-system.svc.cluster.local
    # Istio virtual service host names (optional)
    hosts:
    - app.example.com
  canaryAnalysis:
    # schedule interval (default 60s)
    interval: 1m
    # total number of iterations
    iterations: 10
    # max number of failed iterations before rollback
    threshold: 2
    # canary match condition
    match:
      - headers:
          user-agent:
            regex: "^(?!.*Chrome).*Safari.*"
      - headers:
          cookie:
            regex: "^(.*?;)?(type=insider)(;.*)?$"
    metrics:
    - name: request-success-rate
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      threshold: 99
      interval: 1m
    - name: request-duration
      # maximum req duration P99
      # milliseconds
      threshold: 500
      interval: 30s
    # generate traffic during analysis
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 -H 'Cookie: type=insider' http://podinfo.test:9898/"
```

The above configuration will run an analysis for ten minutes targeting Safari users and those that have an insider cookie.

Save the above resource as podinfo-abtest.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-abtest.yaml
```

After a couple of seconds Flagger will create the canary objects:

```bash
# applied 
deployment.apps/abtest
horizontalpodautoscaler.autoscaling/abtest
canary.flagger.app/abtest

# generated 
deployment.apps/abtest-primary
horizontalpodautoscaler.autoscaling/abtest-primary
service/abtest
service/abtest-canary
service/abtest-primary
destinationrule.networking.istio.io/abtest-canary
destinationrule.networking.istio.io/abtest-primary
virtualservice.networking.istio.io/abtest
```

### Automated canary promotion

Trigger a canary deployment by updating the container image:

```bash
kubectl -n test set image deployment/abtest \
podinfod=stefanprodan/podinfo:2.0.1
```

Flagger detects that the deployment revision changed and starts a new rollout:

```text
kubectl -n test describe canary/abtest

Status:
  Failed Checks:         0
  Phase:                 Succeeded
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    flagger  New revision detected abtest.test
  Normal   Synced  3m    flagger  Scaling up abtest.test
  Warning  Synced  3m    flagger  Waiting for abtest.test rollout to finish: 0 of 1 updated replicas are available
  Normal   Synced  3m    flagger  Advance abtest.test canary iteration 1/10
  Normal   Synced  3m    flagger  Advance abtest.test canary iteration 2/10
  Normal   Synced  3m    flagger  Advance abtest.test canary iteration 3/10
  Normal   Synced  2m    flagger  Advance abtest.test canary iteration 4/10
  Normal   Synced  2m    flagger  Advance abtest.test canary iteration 5/10
  Normal   Synced  1m    flagger  Advance abtest.test canary iteration 6/10
  Normal   Synced  1m    flagger  Advance abtest.test canary iteration 7/10
  Normal   Synced  55s   flagger  Advance abtest.test canary iteration 8/10
  Normal   Synced  45s   flagger  Advance abtest.test canary iteration 9/10
  Normal   Synced  35s   flagger  Advance abtest.test canary iteration 10/10
  Normal   Synced  25s   flagger  Copying abtest.test template spec to abtest-primary.test
  Warning  Synced  15s   flagger  Waiting for abtest-primary.test rollout to finish: 1 of 2 updated replicas are available
  Normal   Synced  5s    flagger  Promotion completed! Scaling down abtest.test
```

**Note** that if you apply new changes to the deployment during the canary analysis, Flagger will restart the analysis.

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        abtest    Progressing   100      2019-03-16T14:05:07Z
prod        frontend  Succeeded     0        2019-03-15T16:15:07Z
prod        backend   Failed        0        2019-03-14T17:05:07Z
```

### Automated rollback

During the canary analysis you can generate HTTP 500 errors and high latency to test Flagger's rollback.

Generate HTTP 500 errors:

```bash
watch curl -b 'type=insider' http://app.example.com/status/500
```

Generate latency:

```bash
watch curl -b 'type=insider' http://app.example.com/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary, 
the canary is scaled to zero and the rollout is marked as failed.

```text
kubectl -n test describe canary/abtest

Status:
  Failed Checks:         2
  Phase:                 Failed
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    flagger  Starting canary deployment for abtest.test
  Normal   Synced  3m    flagger  Advance abtest.test canary iteration 1/10
  Normal   Synced  3m    flagger  Advance abtest.test canary iteration 2/10
  Normal   Synced  3m    flagger  Advance abtest.test canary iteration 3/10
  Normal   Synced  3m    flagger  Halt abtest.test advancement success rate 69.17% < 99%
  Normal   Synced  2m    flagger  Halt abtest.test advancement success rate 61.39% < 99%
  Warning  Synced  2m    flagger  Rolling back abtest.test failed checks threshold reached 2
  Warning  Synced  1m    flagger  Canary failed! Scaling down abtest.test
```
