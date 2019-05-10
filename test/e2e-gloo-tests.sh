#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Helm and NGINX ingress controller

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Creating test namespace'
kubectl create namespace test

echo ">>> Downloading Gloo CLI"
curl -SsL https://github.com/solo-io/gloo/releases/download/v0.13.25/glooctl-linux-amd64 > glooctl
chmod +x glooctl

echo '>>> Installing load tester'
kubectl -n test apply -f ${REPO_ROOT}/artifacts/loadtester/
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml
./glooctl add route --path-prefix / --upstream-group-name podinfo --upstream-group-namespace test

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
  canaryAnalysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
    metrics:
    - name: envoy-success-rate
      threshold: 99
      interval: 1m
      query: |
        sum(rate(envoy_cluster_upstream_rq{kubernetes_namespace="gloo-system", gloo="gateway-proxy", 
            envoy_response_code!~"5.*"}[1m])) 
        / 
        sum(rate(envoy_cluster_upstream_rq{kubernetes_namespace="gloo-system", gloo="gateway-proxy"}[1m]))
        * 100
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -host app.example.com http://gateway-proxy.gloo-system"
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
        kubectl -n gloo-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'
