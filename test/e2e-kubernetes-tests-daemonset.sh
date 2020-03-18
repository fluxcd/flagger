#!/usr/bin/env bash

# This script runs e2e tests for Blue/Green initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Kustomize

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Creating test namespace'
kubectl create namespace test

echo '>>> Installing the load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-daemonset.yaml

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: kubernetes
  targetRef:
    apiVersion: apps/v1
    kind: DaemonSet
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
    name: podinfo-svc
    portDiscovery: true
  analysis:
    interval: 15s
    threshold: 10
    iterations: 5
    metrics:
      - name: request-success-rate
        interval: 1m
        thresholdRange:
          min: 99
      - name: request-duration
        interval: 30s
        thresholdRange:
          max: 500
    webhooks:
      - name: "gate"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/approve
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 10s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-svc-canary/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo-svc-canary.test/"
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

echo '>>> Triggering canary daemonset'
kubectl -n test set image daemonset/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe daemonset/podinfo-primary | grep '3.1.1' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe daemonset/podinfo
        kubectl -n test describe daemonset/podinfo-primary
        kubectl -n flagger-system logs deployment/flagger
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
  revertOnDeletion: true
  provider: kubernetes
  targetRef:
    apiVersion: apps/v1
    kind: DaemonSet
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
    name: podinfo-svc
    portDiscovery: true
  analysis:
    interval: 15s
    threshold: 10
    iterations: 5
    metrics:
      - name: request-success-rate
        interval: 1m
        thresholdRange:
          min: 99
      - name: request-duration
        interval: 30s
        thresholdRange:
          max: 500
    webhooks:
      - name: "gate"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/approve
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 10s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-svc-canary/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo-svc-canary.test/"
          logCmdOutput: "true"
EOF

echo '>>> Waiting for finalizers to be present'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl get canary podinfo -n test -o jsonpath='{.metadata.finalizers}' | grep "finalizer.flagger.app" && ok=true || ok=false
    sleep 10
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe canary/podinfo
        echo "No more retries left"
        exit 1
    fi
done

kubectl delete canary podinfo -n test

echo '>>> Waiting for primary to revert'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl get daemonset podinfo -n test -o jsonpath='{.status.numberReady}' | grep 1 && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe canary/podinfo
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary finalize passed'

kubectl -n flagger-system logs deployment/flagger
