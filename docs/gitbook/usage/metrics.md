# Metrics Analysis

As part of the analysis process, Flagger can validate service level objectives
(SLOs) like availability, error rate percentage, average response time and any other objective
based on app specific metrics.
If a drop in performance is noticed during the SLOs analysis,
the release will be automatically rolled back with minimum impact to end-users.

## Builtin metrics

Flagger comes with two builtin metric checks: HTTP request success rate and duration.

```yaml
  analysis:
    metrics:
    - name: request-success-rate
      interval: 1m
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      thresholdRange:
        min: 99
    - name: request-duration
      interval: 1m
      # maximum req duration P99
      # milliseconds
      thresholdRange:
        max: 500
```

For each metric you can specify a range of accepted values with `thresholdRange` and
the window size or the time series with `interval`.
The builtin checks are available for every service mesh / ingress controller
and are implemented with [Prometheus queries](../faq.md#metrics).

## Custom metrics

The canary analysis can be extended with custom metric checks.
Using a `MetricTemplate` custom resource,
you configure Flagger to connect to a metric provider and run a query that returns a `float64` value.
The query result is used to validate the canary based on the specified threshold range.

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: my-metric
spec:
  provider:
    type: # can be prometheus, datadog, etc
    address: # API URL
    insecureSkipVerify: # if set to true, disables the TLS cert validation
    secretRef:
      name: # name of the secret containing the API credentials
  query: # metric query
```

The following variables are available in query templates:

* `name` (canary.metadata.name)
* `namespace` (canary.metadata.namespace)
* `target` (canary.spec.targetRef.name)
* `service` (canary.spec.service.name)
* `ingress` (canary.spec.ingresRef.name)
* `interval` (canary.spec.analysis.metrics[].interval)
* `variables` (canary.spec.analysis.metrics[].templateVariables)

A canary analysis metric can reference a template with `templateRef`:

```yaml
  analysis:
    metrics:
      - name: "my metric"
        templateRef:
          name: my-metric
          # namespace is optional
          # when not specified, the canary namespace will be used
          namespace: flagger
        # accepted values
        thresholdRange:
          min: 10
          max: 1000
        # metric query time window
        interval: 1m
```

A canary analysis metric can reference a set of custom variables with `templateVariables`. These variables will be then injected into the query defined in the referred `MetricTemplate` object during canary analysis:

```yaml
  analysis:
    metrics:
      - name: "my metric"
        templateRef:
          name: my-metric
          namespace: flagger
        # accepted values
        thresholdRange:
          min: 10
          max: 1000
        # metric query time window
        interval: 1m
        # custom variables used within the referenced metric template
        templateVariables:
          direction: inbound
```

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: my-metric
spec:
  provider:
    type: prometheus
    address: http://prometheus.linkerd-viz:9090
  query: |
    histogram_quantile(
      0.99,
      sum(
        rate(
          response_latency_ms_bucket{
            namespace="{{ namespace }}",
            deployment=~"{{ target }}",
            direction="{{ variables.direction }}"
          }[{{ interval }}]
        )
      ) by (le)
    )
```

## Prometheus

You can create custom metric checks targeting a Prometheus server by
setting the provider type to `prometheus` and writing the query in PromQL.

Prometheus template example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: not-found-percentage
  namespace: istio-system
spec:
  provider:
    type: prometheus
    address: http://prometheus.istio-system:9090
  query: |
    100 - sum(
        rate(
            istio_requests_total{
              reporter="destination",
              destination_workload_namespace="{{ namespace }}",
              destination_workload="{{ target }}",
              response_code!="404"
            }[{{ interval }}]
        )
    )
    /
    sum(
        rate(
            istio_requests_total{
              reporter="destination",
              destination_workload_namespace="{{ namespace }}",
              destination_workload="{{ target }}"
            }[{{ interval }}]
        )
    ) * 100
```

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "404s percentage"
        templateRef:
          name: not-found-percentage
          namespace: istio-system
        thresholdRange:
          max: 5
        interval: 1m
```

The above configuration validates the canary by checking if the HTTP 404 req/sec percentage
is below 5 percent of the total traffic. If the 404s rate reaches the 5% threshold, then the canary fails.

Prometheus gRPC error rate example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: grpc-error-rate-percentage
  namespace: flagger
spec:
  provider:
    type: prometheus
    address: http://flagger-prometheus.flagger-system:9090
  query: |
    100 - sum(
        rate(
            grpc_server_handled_total{
              grpc_code!="OK",
              kubernetes_namespace="{{ namespace }}",
              kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
            }[{{ interval }}]
        )
    )
    /
    sum(
        rate(
            grpc_server_started_total{
              kubernetes_namespace="{{ namespace }}",
              kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"
            }[{{ interval }}]
        )
    ) * 100
```

The above template is for gRPC services instrumented with
[go-grpc-prometheus](https://github.com/grpc-ecosystem/go-grpc-prometheus).

## Prometheus authentication

If your Prometheus API requires basic authentication, you can create a secret in the same namespace
as the `MetricTemplate` with the basic-auth credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: prom-auth
  namespace: flagger
data:
  username: your-user
  password: your-password
```

or if you require bearer token authentication (via a SA token):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: prom-auth
  namespace: flagger
data:
  token: ey1234...
```

Then reference the secret in the `MetricTemplate`:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: my-metric
  namespace: flagger
spec:
  provider:
    type: prometheus
    address: http://prometheus.monitoring:9090
    secretRef:
      name: prom-auth
```

## Datadog

You can create custom metric checks using the Datadog provider.

Create a secret with your Datadog API credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: datadog
  namespace: istio-system
data:
  datadog_api_key: your-datadog-api-key
  datadog_application_key: your-datadog-application-key
```

Datadog template example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: not-found-percentage
  namespace: istio-system
spec:
  provider:
    type: datadog
    address: https://api.datadoghq.com
    secretRef:
      name: datadog
  query: |
    100 - (
      sum:istio.mesh.request.count{
        reporter:destination,
        destination_workload_namespace:{{ namespace }},
        destination_workload:{{ target }},
        !response_code:404
      }.as_count()
      / 
      sum:istio.mesh.request.count{
        reporter:destination,
        destination_workload_namespace:{{ namespace }},
        destination_workload:{{ target }}
      }.as_count()
    ) * 100
```

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "404s percentage"
        templateRef:
          name: not-found-percentage
          namespace: istio-system
        thresholdRange:
          max: 5
        interval: 1m
```

## Amazon CloudWatch

You can create custom metric checks using the CloudWatch metrics provider.

CloudWatch template example:

```yaml
apiVersion: flagger.app/v1alpha1
kind: MetricTemplate
metadata:
  name: cloudwatch-error-rate
spec:
  provider:
    type: cloudwatch
    region: ap-northeast-1 # specify the region of your metrics
  query: |
    [
        {
            "Id": "e1",
            "Expression": "m1 / m2",
            "Label": "ErrorRate"
        },
        {
            "Id": "m1",
            "MetricStat": {
                "Metric": {
                    "Namespace": "MyKubernetesCluster",
                    "MetricName": "ErrorCount",
                    "Dimensions": [
                        {
                            "Name": "appName",
                            "Value": "{{ name }}.{{ namespace }}"
                        }
                    ]
                },
                "Period": 60,
                "Stat": "Sum",
                "Unit": "Count"
            },
            "ReturnData": false
        },
        {
            "Id": "m2",
            "MetricStat": {
                "Metric": {
                    "Namespace": "MyKubernetesCluster",
                    "MetricName": "RequestCount",
                    "Dimensions": [
                        {
                            "Name": "appName",
                            "Value": "{{ name }}.{{ namespace }}"
                        }
                    ]
                },
                "Period": 60,
                "Stat": "Sum",
                "Unit": "Count"
            },
            "ReturnData": false
        }
    ]
```

The query format documentation can be found
[here](https://aws.amazon.com/premiumsupport/knowledge-center/cloudwatch-getmetricdata-api/).

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "app error rate"
        templateRef:
          name: cloudwatch-error-rate
        thresholdRange:
          max: 0.1
        interval: 1m
```

**Note** that Flagger need AWS IAM permission to perform `cloudwatch:GetMetricData` to use this provider.

## New Relic

You can create custom metric checks using the New Relic provider.

Create a secret with your New Relic Insights credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: newrelic
  namespace: istio-system
data:
  newrelic_account_id: your-account-id
  newrelic_query_key: your-insights-query-key
```

New Relic template example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: newrelic-error-rate
  namespace: ingress-nginx
spec:
  provider:
    type: newrelic
    secretRef:
      name: newrelic
  query: |
    SELECT 
        filter(sum(nginx_ingress_controller_requests), WHERE status >= '500') / 
        sum(nginx_ingress_controller_requests) * 100
    FROM Metric 
    WHERE metricName = 'nginx_ingress_controller_requests' 
    AND ingress = '{{ ingress }}' AND  namespace = '{{ namespace }}'
```

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "error rate"
        templateRef:
          name: newrelic-error-rate
          namespace: ingress-nginx
        thresholdRange:
          max: 5
        interval: 1m
```

## Graphite

You can create custom metric checks using the Graphite provider.

Graphite template example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: graphite-request-success-rate
spec:
  provider:
    type: graphite
    address: http://graphite.monitoring
  query: |
    target=summarize(
      asPercent(
        sumSeries(
          stats.timers.httpServerRequests.app.{{target}}.exception.*.method.*.outcome.{CLIENT_ERROR,INFORMATIONAL,REDIRECTION,SUCCESS}.status.*.uri.*.count
        ),
        sumSeries(
          stats.timers.httpServerRequests.app.{{target}}.exception.*.method.*.outcome.*.status.*.uri.*.count
        )
      ),
      {{interval}},
      'avg'
    )
```

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "success rate"
        templateRef:
          name: graphite-request-success-rate
        thresholdRange:
          min: 90
        interval: 1min
```

## Graphite authentication

If your Graphite API requires basic authentication, you can create a secret in the same namespace
as the `MetricTemplate` with the basic-auth credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: graphite-basic-auth
  namespace: flagger
data:
  username: your-user
  password: your-password
```

Then, reference the secret in the `MetricTemplate`:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: my-metric
  namespace: flagger
spec:
  provider:
    type: graphite
    address: http://graphite.monitoring
    secretRef:
      name: graphite-basic-auth
```

## Google Cloud Monitoring (Stackdriver)

Enable Workload Identity on your cluster, create a service account key that has read access to the
Cloud Monitoring API and then create an IAM policy binding between the GCP service account and the Flagger 
service account on Kubernetes. You can take a look at this [guide](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)

Annotate the flagger service account
```shell script
kubectl annotate serviceaccount flagger \
    --namespace <namespace> \
    iam.gke.io/gcp-service-account=<gcp-serviceaccount-name>@<project-id>.iam.gserviceaccount.com
```

Alternatively, you can download the json keys and add it to your secret with the key `serviceAccountKey` (This method is not recommended).

Create a secret that contains your project-id (and, if workload identity is not enabled on your cluster,
your [service account json](https://cloud.google.com/docs/authentication/production#create_service_account)).

```
 kubectl create secret generic gcloud-sa --from-literal=project=<project-id>
```

Then reference the secret in the metric template. 
Note: The particular MQL query used here works if [Istio is installed on GKE](https://cloud.google.com/istio/docs/istio-on-gke/installing).
```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: bytes-sent
  namespace: test
spec:
  provider:
    type: stackdriver
    secretRef: 
      name: gcloud-sa
  query: |
    fetch k8s_container
    | metric 'istio.io/service/server/response_latencies'
    | filter
        (metric.destination_service_name == '{{ service }}-canary'
        && metric.destination_service_namespace == '{{ namespace }}')
    | align delta(1m)
    | every 1m
    | group_by [],
        [value_response_latencies_percentile:
          percentile(value.response_latencies, 99)]
```

The reference for the query language can be found [here](https://cloud.google.com/monitoring/mql/reference)

## InfluxDB

The InfluxDB provider uses the [flux](https://docs.influxdata.com/influxdb/v2.0/query-data/get-started/) query language.

Create a secret that contains your authentication token that can be found in the InfluxDB UI.

```
 kubectl create secret generic influx-token --from-literal=token=<token>
```

Then reference the secret in the metric template.

Note: The particular MQL query used here works if [Istio is installed on GKE](https://cloud.google.com/istio/docs/istio-on-gke/installing).

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: not-found
  namespace: test
spec:
  provider:
    type: influxdb
    secretRef:
      name: influx-token
  query: |
    from(bucket: "default")
    |> range(start: -2h)
    |> filter(fn: (r) => r["_measurement"] == "istio_requests_total")
    |> filter(fn: (r) => r[" destination_workload_namespace"] == "{{ namespace }}")
    |> filter(fn: (r) => r["destination_workload"] == "{{ target }}")
    |> filter(fn: (r) => r["response_code"] == "500")
    |> count()
    |> yield(name: "count")
```

## Dynatrace

You can create custom metric checks using the Dynatrace provider.

Create a secret with your Dynatrace token:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: dynatrace
  namespace: istio-system
data:
  dynatrace_token: ZHQwYz...
```

Dynatrace metric template example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: response-time-95pct
  namespace: istio-system
spec:
  provider:
    type: dynatrace
    address: https://xxxxxxxx.live.dynatrace.com
    secretRef:
      name: dynatrace
  query: |
    builtin:service.response.time:filter(eq(dt.entity.service,SERVICE-ABCDEFG0123456789)):percentile(95)
```

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "response-time-95pct"
        templateRef:
          name: response-time-95pct
          namespace: istio-system
        thresholdRange:
          max: 1000
        interval: 1m
```

## Keptn

You can create custom metric checks using the Keptn provider.
This Provider allows to verify either the value of a single [KeptnMetric](https://keptn.sh/stable/docs/reference/crd-reference/metric/),
representing the value of a single metric,
or of a [Keptn Analysis](https://keptn.sh/stable/docs/reference/crd-reference/analysis/),
which provides a flexible grading logic for analysing and prioritising a number of different
metric values coming from different data sources.

This provider requires [Keptn](https://keptn.sh/stable/docs/installation/) to be installed in the cluster.

Example for a Keptn metric template:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: response-time
  namespace: istio-system
spec:
  provider:
    type: keptn
  query: keptnmetric/my-namespace/response-time/2m/reporter=destination
```

This will reference the `KeptnMetric` with the name `response-time` in
the namespace `my-namespace`, which could look like the following:

```yaml
apiVersion: metrics.keptn.sh/v1beta1
kind: KeptnMetric
metadata:
  name: response-time
  namespace: my-namespace
spec:
  fetchIntervalSeconds: 10
  provider:
    name: my-prometheus-keptn-provider
  query: histogram_quantile(0.8, sum by(le) (rate(http_server_request_latency_seconds_bucket{status_code='200',
    job='simple-go-backend'}[5m[])))
```

The `query` contains the following components, which are divided by `/` characters:

```
<type>/<namespace>/<resource-name>/<timeframe>/<arguments>
```

* **type (required)**: Must be either `keptnmetric` or `analysis`.
* **namespace (required)**: The namespace of the referenced `KeptnMetric`/`AnalysisDefinition`.
* **resource-name (required):** The name of the referenced `KeptnMetric`/`AnalysisDefinition`.
* **timeframe (optional)**: The timeframe used for the Analysis.
This will usually be set to the same value as the analysis interval of a `Canary`.
Only relevant if the `type` is set to `analysis`.
* **arguments (optional)**: Arguments to be passed to an `Analysis`.
Arguments are passed as a list of key value pairs, separated by `;` characters,
e.g. `foo=bar;bar=foo`. 
Only relevant if the `type` is set to `analysis`.

For the type `analysis`, the value returned by the provider is either `0`
(if the analysis failed), or `1` (analysis passed).

## Splunk

You can create custom metric checks using the Splunk provider.

Create a secret that contains your authentication token that can be found in the Splunk o11y UI.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: splunk
  namespace: istio-system
data:
  sf_token_key: your-access-token
```

Splunk template example:

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: success-rate
  namespace: istio-system
spec:
  provider:
    type: splunk
    address: https://api.<REALM>.signalfx.com
    secretRef:
      name: splunk
  query: |
    total = data('traces.count', filter=filter('sf_service', '{{target}}')).sum().publish(enable=False)
    success = data('traces.count', filter=filter('sf_service', '{{target}}') and filter('sf_error', 'false')).sum().publish(enable=False)
    ((success/total) * 100).publish()
```
The query format documentation can be found [here](https://dev.splunk.com/observability/docs/signalflow).

Reference the template in the canary analysis:

```yaml
  analysis:
    metrics:
      - name: "success rate"
        templateRef:
          name: success-rate
          namespace: istio-system
        thresholdRange:
          max: 99
        interval: 1m
```
