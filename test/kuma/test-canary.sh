#!/usr/bin/env bash

# This script runs Kuma e2e tests for Canary initialization, analysis and promotion

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: kuma.io/v1alpha1
kind: TrafficPermission
mesh: default
metadata:
  name: allow-all-traffic
spec:
  sources:
    - match:
        kuma.io/service: '*'
  destinations:
    - match:
        kuma.io/service: '*'
EOF

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
  annotations:
    kuma.io/mesh: default
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    targetPort: 9898
    apex:
      annotations:
        9898.service.kuma.io/protocol: "http"
        ingress.kubernetes.io/service-upstream: "true"
        nginx.ingress.kubernetes.io/service-upstream: "true"
    canary:
      annotations:
        9898.service.kuma.io/protocol: "http"
        ingress.kubernetes.io/service-upstream: "true"
        nginx.ingress.kubernetes.io/service-upstream: "true"
    primary:
      annotations:
        9898.service.kuma.io/protocol: "http"
        ingress.kubernetes.io/service-upstream: "true"
        nginx.ingress.kubernetes.io/service-upstream: "true"
  analysis:
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
    webhooks:
      # temproarily disabled due to upstream issues
      # - name: acceptance-test
      #   type: pre-rollout
      #   url: http://flagger-loadtester.test/
      #   timeout: 30s
      #   metadata:
      #     type: bash
      #     cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 http://podinfo.test:9898/"
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
        kubectl -n kong-mesh-system logs deployment/flagger
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

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n kong-mesh-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n kong-mesh-system logs deployment/flagger
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
        kubectl -n kong-mesh-system logs deployment/flagger
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
  annotations:
    kuma.io/mesh: default
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    targetPort: 9898
    apex:
      annotations:
        9898.service.kuma.io/protocol: "http"
        ingress.kubernetes.io/service-upstream: "true"
        nginx.ingress.kubernetes.io/service-upstream: "true"
    canary:
      annotations:
        9898.service.kuma.io/protocol: "http"
        ingress.kubernetes.io/service-upstream: "true"
        nginx.ingress.kubernetes.io/service-upstream: "true"
    primary:
      annotations:
        9898.service.kuma.io/protocol: "http"
        ingress.kubernetes.io/service-upstream: "true"
        nginx.ingress.kubernetes.io/service-upstream: "true"
  analysis:
    interval: 15s
    threshold: 5
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 30s
    webhooks:
      # temproarily disabled due to upstream issues
      # - name: acceptance-test
      #   type: pre-rollout
      #   url: http://flagger-loadtester.test/
      #   timeout: 30s
      #   metadata:
      #     type: bash
      #     cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 10 -c 2 http://podinfo.test:9898/status/500"
EOF

echo '>>> Triggering canary deployment rollback test'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.2

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n kong-mesh-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n kong-mesh-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary rollback test passed'
