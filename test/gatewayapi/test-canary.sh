#!/usr/bin/env bash

# This script runs e2e tests for Canary, A/B initialization, analysis and promotion
# Prerequisites: Kubernetes Kind and Contour with GatewayAPI

set -o errexit

echo '>>> Create metric templates'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: latency
  namespace: flagger-system
spec:
  provider:
    type: prometheus
    address: http://flagger-prometheus:9090
  query: |
    histogram_quantile(0.99,
      sum(
        rate(
          envoy_cluster_upstream_rq_time_bucket{
            envoy_cluster_name=~"{{ namespace }}_{{ target }}-canary_[0-9a-zA-Z-]+",
          }[{{ interval }}]
        )
      ) by (le)
    )/1000
---
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: error-rate
  namespace: flagger-system
spec:
  provider:
    type: prometheus
    address: http://flagger-prometheus:9090
  query: |
    100 - sum(
      rate(
        envoy_cluster_upstream_rq{
          envoy_cluster_name=~"{{ namespace }}_{{ target }}-canary_[0-9a-zA-Z-]+",
          envoy_response_code!~"5.*"
        }[{{ interval }}]
      )
    )
    /
    sum(
      rate(
        envoy_cluster_upstream_rq{
          envoy_cluster_name=~"{{ namespace }}_{{ target }}-canary_[0-9a-zA-Z-]+",
        }[{{ interval }}]
      )
    )
    * 100
EOF

echo '>>> Installing Canary'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    portName: http
    hosts:
     - localproject.contour.io
    gatewayRefs:
      - name: contour
        namespace: projectcontour
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
      - name: error-rate
        templateRef:
          name: error-rate
          namespace: flagger-system
        thresholdRange:
          max: 1
        interval: 1m
      - name: latency
        templateRef:
          name: latency
          namespace: flagger-system
        thresholdRange:
          max: 0.5
        interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host localproject.contour.io http://envoy.projectcontour/"
          logCmdOutput: "true"
EOF

echo '>>> Waiting for primary to be ready'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Initialized' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'

passed=$(kubectl -n test get svc/podinfo -o jsonpath='{.spec.selector.app}' 2>&1 | { grep podinfo-primary || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 podinfo selector test failed'
  exit 1
fi

echo '✔ Canary service custom metadata test passed'

if ! kubectl -n test get httproute podinfo -oyaml; then
    echo "Could not find HTTPRoute podinfo"
    exit 1
fi

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for canary finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

if [[ "$1" = "canary" ]]; then
  exit 0
fi

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    portName: http
    hosts:
     - localproject.contour.io
    gatewayRefs:
      - name: contour
        namespace: projectcontour
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    metrics:
    - name: error-rate
      templateRef:
        name: error-rate
        namespace: flagger-system
      thresholdRange:
        max: 1
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: flagger-system
      thresholdRange:
        max: 0.5
      interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host localproject.contour.io http://envoy.projectcontour/"
          logCmdOutput: "true"
EOF

echo '>>> Triggering B/G deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.2

echo '>>> Waiting for B/G promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.2' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for B/G finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ B/G promotion test passed'

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  progressDeadlineSeconds: 60
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  service:
    port: 9898
    portName: http
    hosts:
     - localproject.contour.io
    gatewayRefs:
      - name: contour
        namespace: projectcontour
  analysis:
    interval: 15s
    threshold: 10
    iterations: 10
    match:
    - headers:
        x-canary:
          exact: "insider"
    metrics:
    - name: error-rate
      templateRef:
        name: error-rate
        namespace: flagger-system
      thresholdRange:
        max: 1
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: flagger-system
      thresholdRange:
        max: 0.5
      interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host localproject.contour.io http://envoy.projectcontour/"
          logCmdOutput: "true"
EOF

echo '>>> Triggering A/B testing'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.3

echo '>>> Waiting for A/B testing promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.3' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ A/B testing promotion test passed'

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 30
  service:
    port: 9898
    targetPort: 9898
    portName: http
    hosts:
     - localproject.contour.io
    gatewayRefs:
      - name: contour
        namespace: projectcontour
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: error-rate
      templateRef:
        name: error-rate
        namespace: flagger-system
      thresholdRange:
        max: 1
      interval: 30s
    - name: latency
      templateRef:
        name: latency
        namespace: flagger-system
      thresholdRange:
        max: 0.5
      interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host localproject.contour.io http://envoy.projectcontour/status/500"
          logCmdOutput: "true"
EOF

echo '>>> Triggering canary deployment rollback test'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.2

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary rollback test passed'
