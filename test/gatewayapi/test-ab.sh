#!/usr/bin/env bash

# This script runs e2e tests for A/B traffic shifting, Canary analysis and promotion
# Prerequisites: Kubernetes Kind and Contour with GatewayAPI

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

source ${REPO_ROOT}/test/gatewayapi/test-utils.sh

create_latency_metric_template
create_error_rate_metric_template

echo '>>> Deploy podinfo in ab-test namespace'
kubectl create ns ab-test
kubectl apply -f ${REPO_ROOT}/test/workloads/secret.yaml -n ab-test
kubectl apply -f ${REPO_ROOT}/test/workloads/deployment.yaml -n ab-test

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: ab-test
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
    # - name: request-success-rate
      # thresholdRange:
        # min: 99
      # interval: 1m
    # - name: request-duration
      # threshold: 500
      # interval: 30s

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
          cmd: "hey -z 2m -q 10 -c 2 -host localproject.contour.io -H 'X-Canary: insider' http://envoy-contour.projectcontour/"
          logCmdOutput: "true"
EOF

check_primary "ab-test"

display_httproute "ab-test"

echo '>>> Triggering A/B testing'
kubectl -n ab-test set image deployment/podinfo podinfod=stefanprodan/podinfo:6.0.1

echo '>>> Waiting for A/B testing promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n ab-test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n ab-test describe deployment/podinfo
        kubectl -n ab-test describe deployment/podinfo-primary
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

display_httproute "ab-test"

echo '>>> Waiting for A/B finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n ab-test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ A/B testing promotion test passed'

kubectl delete -n ab-test canary podinfo
