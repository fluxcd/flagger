#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: admin.gloo.solo.io/v2
kind: Workspace
metadata:
  labels:
    allow_ingress: "true"
  name: podinfo
  namespace: gloo-mesh
spec:
  workloadClusters:
  - name: cluster1
    namespaces:
    - name: test
---
apiVersion: admin.gloo.solo.io/v2
kind: WorkspaceSettings
metadata:
  name: podinfo
  namespace: test
spec:
  importFrom:
  - workspaces:
    - name: gateways
    resources:
    - kind: SERVICE
  exportTo:
  - workspaces:
    - name: gateways
    resources:
    - kind: SERVICE
      labels:
        expose: "true"
    - kind: ALL
      labels:
        expose: "true"
---
apiVersion: networking.gloo.solo.io/v2
kind: RouteTable
metadata:
  name: podinfo
  namespace: test
  labels:
    expose: "true"
spec:
  hosts:
    - 'podinfo.test.svc.cluster.local'
  virtualGateways:
    - name: north-south-gw
      namespace: istio-gateways
      cluster: cluster1
  http:
    - delegate:
        routeTables:
          - name: podinfo-delegate
            namespace: test
EOF

# Create the canary
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
  labels:
    app: podinfo
spec:
  provider: gloo-mesh
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  routeTableRef:
    name: podinfo
    namespace: test
  service:
    port: 9898
    targetPort: 9898
    apex:
      labels:
        expose: "true"
    canary:
      labels:
        expose: "true"
    primary:
      labels:
        expose: "true"
  progressDeadlineSeconds: 60
  analysis:
    interval: 20s
    threshold: 5
    maxWeight: 50
    stepWeight: 30
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      thresholdRange:
        max: 500
      interval: 30s
    webhooks:
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 http://podinfo-canary.test:9898/"
EOF

echo '>>> Waiting for primary to be ready'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Initialized' && ok=true || ok=false
    if $ok ; then
      kubectl -n test get routetable/podinfo-delegate -ojson| jq '.. | .weight?' | grep 100 && ok=true || ok=false
    fi
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n gloo-mesh logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

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
    kubectl -n gloo-mesh logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n test logs deployment/flagger-loadtester
        kubectl -n gloo-mesh logs deployment/flagger
        kubectl -n gloo-mesh get all
        kubectl -n gloo-mesh get virtualservice podinfo -oyaml
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
        kubectl -n ingress-nginx logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

kubectl -n gloo-mesh logs deployment/flagger

echo '✔ All tests passed'