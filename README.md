# steerer

Istio progressive rollout gated by Prometheus HTTP success rate metric

### Usage

Create a test namespace:

```bash
kubectl apply -f ./artifacts/namespace/
```

Create GA and canary deployments, services and Istio virtual service:

```bash
kubectl apply -f ./artifacts/workloads/
```

Rollout:

![rollout-cli](docs/screens/rollout-cli-output.png)

HTTP success rate query:

```sql
sum(
    irate(
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
    irate(
        istio_requests_total{
          reporter="destination",
          destination_workload_namespace=~"$namespace",
          destination_workload=~"$workload"
        }[$interval]
    )
)
```


