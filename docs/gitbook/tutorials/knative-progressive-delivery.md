# Knative Canary Deployments

This guide shows you how to use [Knative](https://knative.dev/) and Flagger to automate canary deployments.

![Flagger Canary Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-gatewayapi-canary.png)

## Prerequisites

Flagger requires a Kubernetes cluster **v1.19** or newer and a Knative Serving installation that supports
the resources with `serving.knative.dev/v1` as their API version.

Install Knative v1.17.0:

```bash
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.17.0/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.17.0/serving-core.yaml
kubectl apply -f https://github.com/knative/net-kourier/releases/download/knative-v1.17.0/kourier.yaml
kubectl patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress-class":"kourier.ingress.networking.knative.dev"}}'
```


Install Flagger in the `flagger-system` namespace:

```bash
kubectl apply -k github.com/fluxcd/flagger//kustomize/knative
```

Create a namespace for your Kntive Service:

```bash
kubectl create namespace test
```

Create a Knative Service that deploys podinfo:

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: podinfo
  namespace: test
spec:
  template:
    spec:
      containers:
        - image: ghcr.io/stefanprodan/podinfo:6.0.0
          ports:
            - containerPort: 9898
              protocol: TCP
          command:
            - ./podinfo
            - --port=9898
            - --port-metrics=9797
            - --grpc-port=9999
            - --grpc-service-name=podinfo
            - --level=info
            - --random-delay=false
            - --random-error=false
```

Deploy the load testing service to generate traffic during the canary analysis:

```bash
kubectl apply -k https://github.com/fluxcd/flagger//kustomize/tester?ref=main
```

Create a Canary custom resource:

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: knative
  # knative service ref
  targetRef:
    apiVersion: serving.knative.dev/v1
    kind: Service
    name: podinfo
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  analysis:
    # schedule interval (default 60s)
    interval: 15s
    # max number of failed metric checks before rollback
    threshold: 15
    # max traffic percentage routed to canary
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 10
    metrics:
    - name: request-success-rate
      # min success rate (non-5xx responses)
      # percentage (0-100)
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      # milliseconds
      thresholdRange:
         max: 500
      interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 1m -q 5 -c 2 http://podinfo.test"
          logCmdOutput: "true"
```

> Note: Please note that for a Canary resource with `.spec.provider` set to `knative`, the resource is only valid if the
`.spec.targetRef.kind` is `Service` and `.spec.targetRef.apiVersion` is `serving.knative.dev/v1`.

Save the above resource as podinfo-canary.yaml and then apply it:

```bash
kubectl apply -f ./podinfo-canary.yaml
```

When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary.
The canary analysis will run for five minutes while validating the HTTP metrics and rollout hooks every minute.

After a couple of seconds Flagger will make the following changes the Knative Service `podinfo`:

* Add an annotation to the object with the name `flagger.app/primary-revision`.
* Modify the `.spec.traffic` section of the object such that it can manipulate the traffic spread between
  the primary and canary Knative Revision.

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
  Normal   Synced  5s    flagger  Promotion completed! Scaling down podinfo.test
```

A canary deployment is triggered everytime a new Knative Revision is created.

**Note** that if you apply new changes to the Knative Service during the canary analysis, Flagger will restart the analysis.

You can monitor how Flagger progressively changes the Knative Service object to spread traffic between Knative Revisions:

```bash
watch kubectl get httproute -n test podinfo -o=jsonpath='{.spec.traffic}'
```

You can monitor all canaries with:

```bash
watch kubectl get canaries --all-namespaces

NAMESPACE   NAME      STATUS        WEIGHT   LASTTRANSITIONTIME
test        podinfo   Progressing   15       2025-03-16T14:05:07Z
prod        frontend  Succeeded     0        2025-03-16T16:15:07Z
prod        backend   Failed        0        2025-03-16T17:05:07Z
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
watch curl http://podinfo-canary:9898/status/500
```

Generate latency:

```bash
watch curl http://podinfo-canary:9898/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary
Knative Revision and the rollout is marked as failed.

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
