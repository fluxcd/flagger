# steerer

[![travis](https://travis-ci.org/stefanprodan/steerer.svg?branch=master)](https://travis-ci.org/stefanprodan/steerer)
![docker](https://img.shields.io/microbadger/image-size/stefanprodan/steerer.svg)
![license](https://img.shields.io/github/license/stefanprodan/steerer.svg)

Steerer is a Kubernetes operator that automates the promotion of canary deployments
using Istio routing for traffic shifting and Prometheus metrics for canary analysis.

### Install 

Before installing Steerer make sure you have Istio setup up with Prometheus enabled. 
If you are new to Istio you can follow my [GKE service mesh walk-through](https://github.com/stefanprodan/istio-gke).

Deploy Steerer in the `istio-system` using Helm:

```bash
# add Steerer Helm repo
helm repo add steerer https://stefanprodan.github.io/steerer

# install or upgrade Steerer
helm upgrade --install steerer steerer/steerer \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 \
--set controlLoopInterval=1m
```

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
* check canary HTTP success rate
    * halt rollout if percentage is under the specified threshold
* increase canary traffic wight by 10% till it reaches 100% 
    * halt rollout while canary request success rate is under the threshold
    * halt rollout while canary request duration are over the threshold
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
  primary:
    # deployment name
    name: podinfo
    # clusterIP service name
    host: podinfo
  canary:
    name: podinfo-canary
    host: podinfo-canary
  virtualService:
    name: podinfo
    # canary increment step
    # percentage (0-100)
    weight: 10
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
    interval: 1m
```

The canary analysis is using the following promql queries:
 
HTTP requests success rate percentage:

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

HTTP requests milliseconds duration P99:

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

### Example

Create a test namespace with Istio sidecard injection enabled:

```bash
kubectl apply -f ./artifacts/namespaces/
```

Create the primary deployment and service:

```bash
kubectl apply -f ./artifacts/workloads/deployment.yaml
kubectl apply -f ./artifacts/workloads/service.yaml
```

Create the canary deployment, service and horizontal pod auto-scalar:

```bash
kubectl apply -f ./artifacts/workloads/deployment-canary.yaml
kubectl apply -f ./artifacts/workloads/service-canary.yaml
kubectl apply -f ./artifacts/workloads/hpa-canary.yaml
```

Create a virtual service (replace the gateway and the internet domain with your own):

```yaml
kubectl apply -f ./artifacts/workloads/virtual-service.yaml
```

Create a rollout custom resource:

```bash
kubectl apply -f ./artifacts/rollouts/podinfo.yaml
```

Rollout output:

```
kubectl -n test describe rollout/podinfo

Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    steerer  Starting rollout for podinfo.test
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 10
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 20
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 30
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
  Normal   Synced  5s    steerer  Promotion complete! Scaling down podinfo-canary.test
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

