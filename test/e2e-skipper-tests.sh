#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind and Skipper ingress controller

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Creating test namespace'
kubectl create namespace test

echo '>>> Installing load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml
kubectl apply -f ${REPO_ROOT}/test/e2e-skipper-test-ingress.yaml

echo '>>> Create canary CRD'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: skipper
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  ingressRef:
    apiVersion: networking.k8s.io/v1beta1
    kind: Ingress
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: http
  analysis:
    interval: 15s
    threshold: 5
    maxWeight: 40
    stepWeight: 20
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
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 10 -c 2 -host app.example.com http://skipper.kube-system"
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

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
failed=false
until ${ok}; do
  kubectl -n test get canary/podinfo | grep 'Failed' && failed=true || failed=false
  if ${failed}; then
    kubectl -n flagger-system logs deployment/flagger
    echo "Canary failed!"
    exit 1
  fi
  kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
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
