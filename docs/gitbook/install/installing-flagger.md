# Install Flagger

Before installing Flagger make sure you have [Istio](https://istio.io) running with Prometheus enabled. If you are new to Istio you can follow this GKE guide [Istio service mesh walk-through](https://docs.flagger.app/install/install-istio).

**Prerequisites**

* Kubernetes &gt;= 1.9
* Istio &gt;= 1.0
* Prometheus &gt;= 2.6

### Install with Helm and Tiller

Add Flagger Helm repository:

```bash
helm repo add flagger https://flagger.app
```

Deploy Flagger in the _**istio-system**_ namespace:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 \
--set controlLoopInterval=1m
```

Enable **Slack** notifications:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
--set slack.channel=general \
--set slack.user=flagger
```

### Install with kubectl

If you don't have Tiller you can use the helm template command and apply the generated yaml with kubectl:

```bash
# generate
helm template flagger/flagger \
--name flagger \
--namespace=istio-system \
--set metricsServer=http://prometheus.istio-system:9090 \
--set controlLoopInterval=1m > $HOME/flagger.yaml

# apply
kubectl apply -f $HOME/flagger.yaml
```

###  Uninstall

To uninstall/delete the flagger release with Helm run:

```text
helm delete --purge flagger
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

{% hint style="info" %}
On uninstall the Flagger CRD will not be removed. Deleting the CRD will make Kubernetes remove all the objects owned by the CRD like Istio virtual services, Kubernetes deployments and ClusterIP services.
{% endhint %}

If you want to remove all the objects created by Flagger you have delete the canary CRD with kubectl:

```text
kubectl delete crd canaries.flagger.app
```

