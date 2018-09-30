# steerer

[![build](https://travis-ci.org/stefanprodan/steerer.svg?branch=master)](https://travis-ci.org/stefanprodan/steerer)
[![release](https://img.shields.io/github/release/stefanprodan/steerer/all.svg)](https://github.com/stefanprodan/steerer/releases)
[![report](https://goreportcard.com/badge/github.com/stefanprodan/steerer)](https://goreportcard.com/report/github.com/stefanprodan/steerer)
[![license](https://img.shields.io/github/license/stefanprodan/steerer.svg)](https://github.com/stefanprodan/steerer/blob/master/LICENSE)

Steerer is a Kubernetes operator that automates the promotion of canary deployments
using Istio routing for traffic shifting and Prometheus metrics for canary analysis.
The project is currently in experimental phase and it is expected that breaking changes 
to the API will be made in the upcoming releases.

### Install 

Before installing Steerer make sure you have Istio setup up with Prometheus enabled. 
If you are new to Istio you can follow my [Istio service mesh walk-through](https://github.com/stefanprodan/istio-gke).

Deploy Steerer in the `istio-system` namespace using Helm:

```bash
# add the Helm repository
helm repo add steerer https://stefanprodan.github.io/steerer

# install or upgrade
helm upgrade --install steerer steerer/steerer \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 \
--set controlLoopInterval=1m
```

Steerer is compatible with Kubernetes >1.10.0 and Istio >1.0.0.

### Usage

Steerer requires two Kubernetes deployments: one for the version you want to upgrade called _primary_ and one for the _canary_.
Each deployment must have a corresponding ClusterIP service that exposes a port named http or https.
These services are used as destinations in a Istio virtual service.

![steerer-overview](https://github.com/stefanprodan/steerer/blob/master/docs/diagrams/steerer-overview.png)

Gated rollout stages:

* scan for deployments marked for rollout 
* check Istio virtual service routes are mapped to primary and canary ClusterIP services
* check primary and canary deployments status
    * halt rollout if a rolling update is underway
    * halt rollout if pods are unhealthy
* increase canary traffic weight percentage from 0% to 10%
* check canary HTTP request success rate and latency
    * halt rollout if any metric is under the specified threshold
    * increment the failed checks counter
* check if the number of failed checks reached the threshold
    * route all traffic to primary
    * scale to zero the canary deployment and mark it as failed
    * wait for the canary deployment to be updated (revision bump) and start over
* increase canary traffic wight by 10% (step wight) till it reaches 100% (max weight) 
    * halt rollout while canary request success rate is under the threshold
    * halt rollout while canary request duration P99 is over the threshold
    * halt rollout if the primary or canary deployment becomes unhealthy 
    * halt rollout while canary deployment is being scaled up/down by HPA
* promote canary to primary
    * copy canary deployment spec template over primary
* wait for primary rolling update to finish
    * halt rollout if pods are unhealthy
* route all traffic to primary
* scale to zero the canary deployment
* mark rollout as finished
* wait for the canary deployment to be updated (revision bump) and start over

You can change the canary analysis _max weight_ and the _step wight_ percentage in the rollout custom resource.

Assuming the primary deployment is named _podinfo_ and the canary one _podinfo-canary_, Steerer will require 
a virtual service configured with weight-based routing:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: podinfo
spec:
  hosts:
  - podinfo
  http:
  - route:
    - destination:
        host: podinfo
        port:
          number: 9898
      weight: 100
    - destination:
        host: podinfo-canary
        port:
          number: 9898
      weight: 0
```

Primary and canary services should expose a port named http:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: podinfo-canary
spec:
  type: ClusterIP
  selector:
    app: podinfo-canary
  ports:
  - name: http
    port: 9898
    targetPort: 9898
```

Based on the two deployments, services and virtual service, a rollout can be defined using Steerer's custom resource:

```yaml
apiVersion: apps.weave.works/v1beta1
kind: Rollout
metadata:
  name: podinfo
  namespace: test
spec:
  targetKind: Deployment
  virtualService:
    name: podinfo
  primary:
    name: podinfo
    host: podinfo
  canary:
    name: podinfo-canary
    host: podinfo-canary
  canaryAnalysis:
    # max number of failed checks
    # before rolling back the canary
    threshold: 10
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 100
    # canary increment step
    # percentage (0-100)
    stepWeight: 10
    metrics:
    - name: istio_requests_total
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      threshold: 99
      interval: 1m
    - name: istio_request_duration_seconds_bucket
      # maximum req duration P99
      # milliseconds
      threshold: 500
      interval: 30s
```

The canary analysis is using the following promql queries:
 
_HTTP requests success rate percentage_

```sql
sum(
    rate(
        istio_requests_total{
          reporter="destination",
          destination_workload_namespace=~"$namespace",
          destination_workload=~"$workload",
          response_code!~"5.*"
        }[$interval]
    )
) 
/ 
sum(
    rate(
        istio_requests_total{
          reporter="destination",
          destination_workload_namespace=~"$namespace",
          destination_workload=~"$workload"
        }[$interval]
    )
)
```

_HTTP requests milliseconds duration P99_

```sql
histogram_quantile(0.99, 
  sum(
    irate(
      istio_request_duration_seconds_bucket{
        reporter="destination",
        destination_workload=~"$workload",
        destination_workload_namespace=~"$namespace"
      }[$interval]
    )
  ) by (le)
)
```

### Automated canary analysis, promotions and rollbacks

![steerer-canary](https://github.com/stefanprodan/steerer/blob/master/docs/diagrams/steerer-canary-hpa.png)

Create a test namespace with Istio sidecar injection enabled:

```bash
export REPO=https://raw.githubusercontent.com/stefanprodan/steerer/master

kubectl apply -f ${REPO}/artifacts/namespaces/test.yaml
```

Create the primary deployment, service and hpa:

```bash
kubectl apply -f ${REPO}/artifacts/workloads/primary-deployment.yaml
kubectl apply -f ${REPO}/artifacts/workloads/primary-service.yaml
kubectl apply -f ${REPO}/artifacts/workloads/primary-hpa.yaml
```

Create the canary deployment, service and hpa:

```bash
kubectl apply -f ${REPO}/artifacts/workloads/canary-deployment.yaml
kubectl apply -f ${REPO}/artifacts/workloads/canary-service.yaml
kubectl apply -f ${REPO}/artifacts/workloads/canary-hpa.yaml
```

Create a virtual service (replace the Istio gateway and the internet domain with your own):

```bash
kubectl apply -f ${REPO}/artifacts/workloads/virtual-service.yaml
```

Create a rollout custom resource:

```bash
kubectl apply -f ${REPO}/artifacts/rollouts/podinfo.yaml
```

Rollout output:

```
kubectl -n test describe rollout/podinfo

Status:
  Canary Revision:  16271121
  Failed Checks:    6
  State:            finished
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    steerer  Starting rollout for podinfo.test
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 10
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 20
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 30
  Warning  Synced  3m    steerer  Halt rollout podinfo.test request duration 2.525s > 500ms
  Warning  Synced  3m    steerer  Halt rollout podinfo.test request duration 1.567s > 500ms
  Warning  Synced  3m    steerer  Halt rollout podinfo.test request duration 823ms > 500ms
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 40
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 50
  Normal   Synced  1m    steerer  Advance rollout podinfo.test weight 60
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 82.33% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 87.22% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 94.74% < 99%
  Normal   Synced  1m    steerer  Advance rollout podinfo.test weight 70
  Normal   Synced  55s   steerer  Advance rollout podinfo.test weight 80
  Normal   Synced  45s   steerer  Advance rollout podinfo.test weight 90
  Normal   Synced  35s   steerer  Advance rollout podinfo.test weight 100
  Normal   Synced  25s   steerer  Copying podinfo-canary.test template spec to podinfo.test
  Warning  Synced  15s   steerer  Waiting for podinfo.test rollout to finish: 1 of 2 updated replicas are available
  Normal   Synced  5s    steerer  Promotion completed! Scaling down podinfo-canary.test
```

During the rollout you can generate HTTP 500 errors and high latency to test if Steerer pauses the rollout.

Create a tester pod and exec into it:

```bash
kubectl -n test run tester --image=quay.io/stefanprodan/podinfo:1.2.1 -- ./podinfo --port=9898
kubectl -n test exec -it tester-xx-xx sh
```

Generate HTTP 500 errors:

```bash
watch curl http://podinfo-canary:9898/status/500
```

Generate latency:

```bash
watch curl http://podinfo-canary:9898/delay/1
```

When the number of failed checks reaches the canary analysis threshold, the traffic is routed back to the primary,
the canary is scaled to zero and the rollout is marked as failed. 

```
kubectl -n test describe rollout/podinfo

Status:
  Canary Revision:  16695041
  Failed Checks:    10
  State:            failed
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    steerer  Starting rollout for podinfo.test
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 10
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 20
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 30
  Normal   Synced  3m    steerer  Halt rollout podinfo.test success rate 69.17% < 99%
  Normal   Synced  2m    steerer  Halt rollout podinfo.test success rate 61.39% < 99%
  Normal   Synced  2m    steerer  Halt rollout podinfo.test success rate 55.06% < 99%
  Normal   Synced  2m    steerer  Halt rollout podinfo.test success rate 47.00% < 99%
  Normal   Synced  2m    steerer  (combined from similar events): Halt rollout podinfo.test success rate 38.08% < 99%
  Warning  Synced  1m    steerer  Rolling back podinfo-canary.test failed checks threshold reached 10
  Warning  Synced  1m    steerer  Canary failed! Scaling down podinfo.test
```

Trigger a new rollout by updating the canary image:

```bash
kubectl -n test set image deployment/podinfo-canary \
podinfod=quay.io/stefanprodan/podinfo:1.2.1

kubectl -n test scale deployment/podinfo-canary \
--replicas=1
```

Steer detects that the canary revision changed and starts a new rollout:

```
kubectl -n test describe rollout/podinfo

Status:
  Canary Revision:  16871136
  Failed Checks:    0
  State:            finished
Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    steerer  Waiting for podinfo.test rollout to finish: 0 of 1 updated replicas are available
  Normal   Synced  3m    steerer  Starting rollout for podinfo.test
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 10
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 20
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 30
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 40
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 50
  Normal   Synced  1m    steerer  Advance rollout podinfo.test weight 60
  Normal   Synced  1m    steerer  Advance rollout podinfo.test weight 70
  Normal   Synced  55s   steerer  Advance rollout podinfo.test weight 80
  Normal   Synced  45s   steerer  Advance rollout podinfo.test weight 90
  Normal   Synced  35s   steerer  Advance rollout podinfo.test weight 100
  Normal   Synced  25s   steerer  Copying podinfo-canary.test template spec to podinfo.test
  Warning  Synced  15s   steerer  Waiting for podinfo.test rollout to finish: 1 of 2 updated replicas are available
  Normal   Synced  5s    steerer  Promotion completed! Scaling down podinfo-canary.test
```

### Contributing

Steerer is Apache 2.0 licensed and accepts contributions via GitHub pull requests.

When submitting bug reports please include as much details as possible: 
* which Steerer version
* which Steerer CRD version
* which Kubernetes/Istio version
* what configuration (rollout, virtual service and workloads definitions)
* what happened (Steerer, Istio Pilot and Proxy logs)
