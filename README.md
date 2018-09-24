# steerer

Istio progressive rollout gated by Prometheus HTTP success rate metric

### Usage

Deploy steerer in `istio-system` namespace:

```bash
kubectl apply -f ./artifacts/steerer
```

Create a test namespace:

```bash
kubectl apply -f ./artifacts/namespace/
```

Create GA and canary deployments, services, hpa and Istio virtual service:

```bash
kubectl apply -f ./artifacts/workloads/
```

Create rollout custom resources:

```bash
kubectl apply -f ./artifacts/rollouts/
```

Rollout output:

```bash
kubectl -n test describe rollout/podinfo

Events:
  Type     Reason  Age   From     Message
  ----     ------  ----  ----     -------
  Normal   Synced  3m    steerer  Stating rollout for podinfo.test
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 10
  Normal   Synced  3m    steerer  Advance rollout podinfo.test weight 20
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 30
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 40
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 50
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 60
  Normal   Synced  2m    steerer  Advance rollout podinfo.test weight 60
  Warning  Synced  2m    steerer  Halt rollout podinfo.test success rate 88.89% < 99%
  Warning  Synced  2m    steerer  Halt rollout podinfo.test success rate 82.86% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 80.49% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 82.98% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 83.33% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 82.22% < 99%
  Warning  Synced  1m    steerer  Halt rollout podinfo.test success rate 94.74% < 99%
  Normal   Synced  1m    steerer  Advance rollout podinfo.test weight 70
  Normal   Synced  55s   steerer  Advance rollout podinfo.test weight 80
  Normal   Synced  45s   steerer  Advance rollout podinfo.test weight 90
  Normal   Synced  35s   steerer  Advance rollout podinfo.test weight 100
  Normal   Synced  25s   steerer  Copying podinfo-canary.test template spec to podinfo.test
  Warning  Synced  15s   steerer  Waiting for podinfo.test rollout to finish: 1 of 2 updated replicas are available
  Normal   Synced  5s    steerer  Promotion complete! Scaling down podinfo-canary.test
```

Rollout flow:

* scan for deployments marked for rollout 
* check Istio virtual service routes are mapped to primary and canary ClusterIP services
* check primary and canary deployments status
    * halt rollout if a rolling update is underway
    * halt rollout if pods are unhealthy
* increase canary traffic weight percentage by 10%
* check canary HTTP success rate
    * halt rollout if percentage is under the specified threshold
* increase canary traffic wight by 10% till it reaches 100% 
    * halt rollout while canary success rate is under the threshold
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

HTTP success rate query:

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


