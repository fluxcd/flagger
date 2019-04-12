#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Helm and Istio

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Creating test namespace'
kubectl create namespace test
kubectl label namespace test istio-injection=enabled

echo '>>> Installing the load tester'
kubectl -n test apply -f ${REPO_ROOT}/artifacts/loadtester/
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1alpha3
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
    headers:
      request:
        add:
          x-envoy-upstream-rq-timeout-ms: "15000"
          x-envoy-max-retries: "10"
          x-envoy-retry-on: "gateway-error,connect-failure,refused-stream"
  canaryAnalysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
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
kubectl -n test set image deployment/podinfo podinfod=quay.io/stefanprodan/podinfo:1.4.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '1.4.1' && ok=true || ok=false
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

echo '✔ Canary promotion test passed'

kubectl -n istio-system logs deployment/flagger

echo '✔ All tests passed'