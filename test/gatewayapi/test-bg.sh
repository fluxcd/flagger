#!/usr/bin/env bash

# This script runs e2e tests for Blue/Green traffic shifting, Canary analysis and promotion
# Prerequisites: Kubernetes Kind and Contour with GatewayAPI

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

source ${REPO_ROOT}/test/gatewayapi/test-utils.sh

echo '>>> Deploy podinfo in bg-test namespace'
kubectl create ns bg-test
kubectl apply -f ${REPO_ROOT}/test/workloads/secret.yaml -n bg-test
kubectl apply -f ${REPO_ROOT}/test/workloads/deployment.yaml -n bg-test

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: bg-test
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
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 30s
    webhooks:
      - name: smoke-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'anon' http://podinfo-canary.bg-test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 http://podinfo-canary.bg-test:9898"
          logCmdOutput: "true"
EOF
 
check_primary "bg-test"

display_httproute "bg-test"

echo '>>> Triggering B/G deployment'
kubectl -n bg-test set image deployment/podinfo podinfod=stefanprodan/podinfo:6.0.1

echo '>>> Waiting for B/G promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n bg-test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n bg-test describe deployment/podinfo
        kubectl -n bg-test describe deployment/podinfo-primary
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

display_httproute "bg-test"

echo '>>> Waiting for B/G finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n bg-test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ B/G promotion test passed'

kubectl delete -n bg-test canary podinfo
