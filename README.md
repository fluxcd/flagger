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

Start rollout:

![rollout-cli](docs/screens/rollout-cli-output.png)


