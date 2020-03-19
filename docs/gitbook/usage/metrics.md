# Metrics Analysis

As part of the analysis process, Flagger can validate service level objectives (SLOs) like
availability, error rate percentage, average response time and any other objective based on app specific metrics.
If a drop in performance is noticed during the SLOs analysis,
the release will be automatically rolled back with minimum impact to end-users.

### Builtin metrics

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

For each metric you can specify a range of accepted values with `thresholdRange`
and the window size or the time series with `interval`.
The builtin checks are available for every service mesh / ingress controller
and are implemented with [Prometheus queries](../faq.md#metrics).

### Custom metrics

The canary analysis can be extended with custom metric checks. Using a `MetricTemplate` custom resource, you 
configure Flagger to connect to a metric provider and run a query that returns a `float64` value.
The query result is used to validate the canary based on the specified threshold range.

```yaml
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: my-metric
spec:
  provider:
    type: # can be prometheus or datadog
    address: # API URL
    secretRef:
      name: # name of the secret containing the API credentials
  query: # metric query
```

The following variables are available in query templates:

- `name` (canary.metadata.name)
- `namespace` (canary.metadata.namespace)
- `target` (canary.spec.targetRef.name)
- `service` (canary.spec.service.name)
- `ingress` (canary.spec.ingresRef.name)
- `interval` (canary.spec.analysis.metrics[].interval)

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

### Prometheus 

You can create custom metric checks targeting a Prometheus server
by setting the provider type to `prometheus` and writing the query in PromQL.

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
    address: http://promethues.istio-system:9090
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

The above configuration validates the canary by checking
if the HTTP 404 req/sec percentage is below 5 percent of the total traffic.
If the 404s rate reaches the 5% threshold, then the canary fails.

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
    address: http://flagger-promethues.flagger-system:9090
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

The above template is for gRPC services instrumented with [go-grpc-prometheus](https://github.com/grpc-ecosystem/go-grpc-prometheus).

### Datadog

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


### Amazon CloudWatch

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

The query format documentation can be found [here](https://aws.amazon.com/premiumsupport/knowledge-center/cloudwatch-getmetricdata-api/).

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
