# Steerer

Steerer is a Kubernetes operator that automates the promotion of canary deployments
using Istio routing for traffic shifting and Prometheus metrics for canary analysis.

### Install 

```console
$ helm repo add steerer https://stefanprodan.github.io/steerer
$ helm upgrade --install steerer steerer/steerer --namespace=istio-system
```

Docs: [github.com/stefanprodan/steerer](https://github.com/stefanprodan/steerer)
