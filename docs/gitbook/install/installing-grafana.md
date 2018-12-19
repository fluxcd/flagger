---
description: Helm setup instructions
---

# Installing Grafana

Flagger comes with a Grafana dashboard made for monitoring the canary analysis.

### Install with Helm and Tiller

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Deploy Grafana in the _**istio-system**_ namespace:

```bash
helm upgrade -i flagger-grafana flagger/grafana \
--namespace=istio-system \
--set url=http://prometheus:9090 \
--set user=admin \
--set password=admin
```

### Install with kubectl

If you don't have Tiller you can use the helm template command and apply the generated yaml with kubectl:

```bash
# generate
helm template flagger/grafana \
--name flagger-grafana \
--namespace=istio-system \
--set user=admin \
--set password=admin > $HOME/flagger-grafana.yaml

# apply
kubectl apply -f $HOME/flagger-grafana.yaml
```

###  Uninstall

To uninstall/delete the Grafana release with Helm run:

```text
helm delete --purge flagger-grafana
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

