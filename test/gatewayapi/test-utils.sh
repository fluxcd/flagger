#!/usr/bin/env bash

check_primary() {
    echo '>>> Waiting for primary to be ready'
    namespace=$1
    retries=50
    count=0
    ok=false
    until ${ok}; do
        kubectl get -n ${namespace} canary/podinfo | grep 'Initialized' && ok=true || ok=false
        sleep 5
        count=$(($count + 1))
        if [[ ${count} -eq ${retries} ]]; then
            kubectl -n flagger-system logs deployment/flagger
            echo "No more retries left"
            exit 1
        fi
    done

    echo '✔ Canary initialization test passed'

    passed=$(kubectl -n $namespace get svc/podinfo -o jsonpath='{.spec.selector.app}' 2>&1 | { grep podinfo-primary || true; })
    if [ -z "$passed" ]; then
      echo -e '\u2716 podinfo selector test failed'
      exit 1
    fi

    echo '✔ Canary service custom metadata test passed'
}

display_httproute() {
    namespace=$1
    if ! kubectl -n ${namespace} get httproute podinfo -oyaml; then
        echo "Could not find HTTPRoute podinfo in ${namespace} namespace"
        exit 1
    fi
}

create_request_duration_metric_template() {
    if ! kubectl -n flagger-system get metrictemplates request-duration ; then
        echo '>>> Create request-duration metric template'
        cat <<EOF | kubectl apply -f -
        apiVersion: flagger.app/v1beta1
        kind: MetricTemplate
        metadata:
          name: request-duration
          namespace: flagger-system
        spec:
          provider:
            type: prometheus
            address: http://prometheus.istio-system:9090
          query: |
            histogram_quantile(0.99,
              sum(
                rate(
                  http_request_duration_seconds_bucket{
                    namespace=~"{{ namespace }}",
                    app="{{ target }}",
                  }[{{ interval }}]
                )
              ) by (le)
            )
EOF
    fi
}

create_latency_metric_template() {
    if ! kubectl -n flagger-system get metrictemplates latency; then
        echo '>>> Create latency metric template'
        cat <<EOF | kubectl apply -f -
        apiVersion: flagger.app/v1beta1
        kind: MetricTemplate
        metadata:
          name: latency
          namespace: flagger-system
        spec:
          provider:
            type: prometheus
            address: http://prometheus.istio-system:9090
          query: |
            histogram_quantile(0.99,
              sum(
                rate(
                  istio_request_duration_milliseconds_bucket{
                    reporter="source",
                    destination_workload_namespace=~"{{ namespace }}",
                    destination_workload=~"{{ target }}",
                  }[{{ interval }}]
                )
              ) by (le)
            )/1000
EOF
    fi
}

create_error_rate_metric_template() {
    if ! kubectl -n flagger-system get metrictemplates error-rate; then
        echo '>>> Create latency metric template'
        cat <<EOF | kubectl apply -f -
        apiVersion: flagger.app/v1beta1
        kind: MetricTemplate
        metadata:
          name: error-rate
          namespace: flagger-system
        spec:
          provider:
            type: prometheus
            address: http://prometheus.istio-system:9090
          query: |
            100 - sum(
              rate(
                istio_requests_total{
                  reporter="source",
                  destination_workload_namespace=~"{{ namespace }}",
                  destination_workload=~"{{ target }}",
                  response_code!~"5.*"
                }[{{ interval }}]
              )
            )
            /
            sum(
              rate(
                istio_requests_total{
                  reporter="source",
                  destination_workload_namespace=~"{{ namespace }}",
                  destination_workload=~"{{ target }}",
                }[{{ interval }}]
              )
            )
            * 100
EOF
    fi
}
