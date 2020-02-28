#!/usr/bin/env bash

# This script runs e2e tests for Canary, B/G and A/B initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Helm and Istio

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Creating test namespace'
kubectl create namespace test
kubectl label namespace test istio-injection=enabled

echo '>>> Installing the load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml

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
    portDiscovery: true
    headers:
      request:
        add:
          x-envoy-upstream-rq-timeout-ms: "15000"
          x-envoy-max-retries: "10"
          x-envoy-retry-on: "gateway-error,connect-failure,refused-stream"
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 30s
    - name: "404s percentage"
      threshold: 5
      interval: 1m
      query: |
        100 - sum(
            rate(
                istio_requests_total{
                  reporter="destination",
                  destination_workload_namespace=~"test",
                  destination_workload=~"podinfo",
                  response_code!="404"
                }[1m]
            )
        )
        /
        sum(
            rate(
                istio_requests_total{
                  reporter="destination",
                  destination_workload_namespace=~"test",
                  destination_workload=~"podinfo"
                }[1m]
            )
        ) * 100
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo.test:9898/"
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
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
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
        kubectl -n istio-system logs deployment/flagger
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
    portDiscovery: true
    port: 80
    targetPort: 9898
    portName: http-podinfo
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 30s
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
        timeout: 30s
        metadata:
          type: bash
          cmd: "grpc_health_probe -connect-timeout=1s -addr=podinfo-canary:9999"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 5m -q 10 -c 2 http://podinfo-canary.test/"
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
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
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
        kubectl -n istio-system logs deployment/flagger
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
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    portDiscovery: true
    port: 80
    portName: http-podinfo
    targetPort: http
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    match:
      - headers:
          cookie:
            regex: "^(.*?;)?(type=insider)(;.*)?$"
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 30s
    webhooks:
      - name: pre
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -H 'Cookie: type=insider' http://podinfo-canary.test/"
          logCmdOutput: "true"
      - name: promote-gate
        type: confirm-promotion
        url: http://flagger-loadtester.test/gate/approve
      - name: post
        type: post-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: cmd
          cmd: "curl -s http://podinfo.test/"
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
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ A/B testing promotion test passed'

kubectl -n istio-system logs deployment/flagger

echo '✔ All tests passed'
