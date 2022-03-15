#!/usr/bin/env bash

# This script runs e2e tests for Blue/Green initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Kustomize

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: podinfo-svc
  namespace: test
spec:
  type: ClusterIP
  selector:
    app: podinfo
  ports:
    - name: http
      port: 9898
      protocol: TCP
      targetPort: http
---
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: kubernetes
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
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

passed=$(kubectl -n test get svc/podinfo-svc -oyaml 2>&1 | { grep 'kustomize.toolkit.fluxcd.io/reconcile' || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 toolkit annotation test failed'
  kubectl -n test get svc/podinfo-svc -oyaml
  exit 1
fi
passed=$(kubectl -n test get deploy/podinfo-primary -oyaml 2>&1 | { grep test-label-prefix || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 primary copy labels by prefix test failed'
  exit 1
fi
passed=$(kubectl -n test get deploy/podinfo-primary -oyaml 2>&1 | { grep test-annotation-prefix || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 primary copy annotations by prefix test failed'
  exit 1
fi

echo '✔ Canary initialization test passed'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
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
