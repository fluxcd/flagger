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
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml

kubectl apply -n test -f - <<EOS
apiVersion: v1
kind: Service
metadata:
  name: podinfo
spec:
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: http
  selector:
    app: podinfo
  type: ClusterIP
EOS

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: kubernetes
  targetRef:
    apiVersion: core/v1
    kind: Service
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
  analysis:
    interval: 15s
    threshold: 10
    iterations: 5
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 30s
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
          cmd: "curl -sd 'test' http://podinfo-canary/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo-canary.test/"
          logCmdOutput: "true"
EOF

echo '>>> Waiting for primary to be ready'
kubectl -n test rollout status deploy/podinfo

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

echo '>>> Initializing secondary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload-v2.yaml

echo '>>> Waiting for secondary to be ready'
kubectl -n test rollout status deploy/podinfo-v2

echo '>>> Triggering canary deployment'
kubectl apply -n test -f - <<EOS
apiVersion: v1
kind: Service
metadata:
  name: podinfo
spec:
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: http
  selector:
    app: podinfo-v2
  type: ClusterIP
EOS

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe service/podinfo-primary | grep 'podinfo-v2' && ok=true || ok=false
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

echo '✔ Canary promotion test passed'

kubectl -n flagger-system logs deployment/flagger
