# Metrics Analysis

As part of the analysis process, Flagger can validate service level objectives (SLOs) like
availability, error rate percentage, average response time and any other objective based on app specific metrics.
If a drop in performance is noticed during the SLOs analysis,
the release will be automatically rolled back with minimum impact to end-users.

### Builtin Metrics

Flagger comes with two builtin metric checks: HTTP request success rate and duration.

```yaml
  canaryAnalysis:
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

### Custom Metrics

The canary analysis can be extended with custom metric checks. Using a `MetricTemplate` custom resource, you 
configure Flagger to connect to a metric provider and run a query that returns a `float64` value.
The query result is used to validate the canary based on the specified threshold range.

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

The following variables are available in templates:

- `name` (canary.metadata.name)
- `namespace` (canary.metadata.namespace)
- `target` (canary.spec.targetRef.name)
- `service` (canary.spec.service.name)
- `ingress` (canary.spec.ingresRef.name)
- `interval` (canary.spec.canaryAnalysis.metrics[].interval)

A canary analysis metric can reference a template with `templateRef`:

```yaml
  canaryAnalysis:
    metrics:
      - name: "404s percentage"
        templateRef:
          name: not-found-percentage
          # namespace is optional
          # when not specified, the canary namespace will be used
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

The above template is for gPRC services instrumented with [go-grpc-prometheus](https://github.com/grpc-ecosystem/go-grpc-prometheus).