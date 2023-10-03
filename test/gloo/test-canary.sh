#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: podinfo
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - app.example.com
    routes:
      - matchers:
         - prefix: /
        delegateAction:
          ref:
            name: podinfo
            namespace: test
EOF

# Create upstream that will have config that will be applied to generated flagger upstreams
# but will be used for no other reason
cat <<EOF | kubectl apply -f -
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: config-upstream
  namespace: gloo-system
spec:
  static:
    hosts:
    - addr: "example.com"
      port: 80
  connectionConfig:
    connectTimeout: 2s
    maxRequestsPerConnection: 51
  loadBalancerConfig:
    roundRobin:
      slowStartConfig:
        slowStartWindow: 2m
        minWeightPercent: 5
EOF

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  upstreamRef:
    apiVersion: gloo.solo.io/v1
    kind: Upstream
    name: config-upstream
    namespace: gloo-system
  provider: gloo
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    webhooks:
      - name: "gate"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/approve
      - name: gloo-acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 10s
        metadata:
          type: bash
          cmd: "curl -sd 'test' -H 'Host: app.example.com' http://gateway-proxy.gloo-system/token | grep token"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 5 -c 2 -host app.example.com http://gateway-proxy.gloo-system"
EOF

echo '>>> Waiting for primary to be ready'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Initialized' && ok=true || ok=false
    if $ok ; then
      kubectl -n test get upstream/test-podinfo-canaryupstream-80 -ojson | jq 'any(.spec.connectionConfig.maxRequestsPerConnection; contains(51))' && ok=true || ok=false
    fi
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n gloo-system logs deployment/flagger
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
    kubectl -n gloo-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n test logs deployment/flagger-loadtester
        kubectl -n gloo-system logs deployment/flagger
        kubectl -n gloo-system get all
        kubectl -n gloo-system get virtualservice podinfo -oyaml
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

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: gloo
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    match:
      - headers:
          x-canary:
            exact: "insider"
    metrics:
    - name: request-success-rate
      threshold: 99
      interval: 1m
    - name: request-duration
      threshold: 500
      interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 1m -q 5 -c 2 -H 'X-Canary: insider' -host app.example.com http://gateway-proxy.gloo-system"
          logCmdOutput: "true"
EOF

echo '>>> Triggering A/B testing'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.2

echo '>>> Waiting for A/B testing promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.2' && ok=true || ok=false
    sleep 10
    kubectl -n gloo-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n gloo-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ A/B testing promotion test passed'

kubectl -n gloo-system logs deployment/flagger

echo '✔ All tests passed'
