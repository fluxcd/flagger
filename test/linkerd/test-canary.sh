#!/usr/bin/env bash

# This script runs Linkerd e2e tests for Canary initialization, analysis and promotion

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: success-rate
  namespace: linkerd
spec:
  provider:
    type: prometheus
    address: http://prometheus.linkerd-viz:9090
  query: |
    sum(
      rate(
        response_total{
          namespace="{{ namespace }}",
          deployment=~"{{ target }}",
          classification!="failure",
          direction="{{ variables.direction }}"
        }[{{ interval }}]
      )
    ) 
    / 
    sum(
      rate(
        response_total{
          namespace="{{ namespace }}",
          deployment=~"{{ target }}",
          direction="{{ variables.direction }}"
        }[{{ interval }}]
      )
    ) 
    * 100
EOF

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: latency
  namespace: linkerd
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
EOF

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
    port: 80
    targetPort: http
    portDiscovery: true
    gatewayRefs:
      - name: podinfo
        namespace: test
        group: core
        kind: Service
        port: 80
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: success-rate
      templateRef:
        name: success-rate
        namespace: linkerd
      threshold: 99
      interval: 1m
      templateVariables:
        direction: inbound
    - name: latency
      templateRef:
        name: latency
        namespace: linkerd
      threshold: 300
      interval: 1m
      templateVariables:
        direction: inbound
    webhooks:
      - name: http-acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary/token | grep token"
      - name: grpc-acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: bash
          cmd: "grpc_health_probe -connect-timeout=1s -addr=podinfo-canary:9999"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo.test/"
          logCmdOutput: "true"
EOF

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo-service
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo-service
  progressDeadlineSeconds: 60
  service:
    port: 9898
    portDiscovery: true
    gatewayRefs:
      - name: podinfo
        namespace: test
        group: core
        kind: Service
        port: 9898
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
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
        kubectl -n flagger-system logs deployment/flagger flagger
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
passed=$(kubectl -n test get svc/podinfo-service-canary -o jsonpath='{.spec.selector.app}' 2>&1 | { grep podinfo || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 podinfo-service selector test failed'
  exit 1
fi

echo '✔ Canary service custom metadata test passed'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger flagger
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
        kubectl -n flagger-system logs deployment/flagger flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

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
    port: 80
    targetPort: 9898
    gatewayRefs:
      - name: podinfo
        namespace: test
        group: core
        kind: Service
        port: 80
  analysis:
    interval: 15s
    threshold: 3
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: success-rate
      templateRef:
        name: success-rate
        namespace: linkerd
      threshold: 99
      interval: 1m
      templateVariables:
        direction: inbound
    - name: latency
      templateRef:
        name: latency
        namespace: linkerd
      threshold: 500
      interval: 30s
      templateVariables:
        direction: inbound
    webhooks:
      - name: http-acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo.test/status/500"
EOF

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.2

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary rollback test passed'
