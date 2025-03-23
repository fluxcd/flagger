#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind and Knative Serving

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: knative
  targetRef:
    apiVersion: serving.knative.dev/v1
    kind: Service
    name: podinfo
  progressDeadlineSeconds: 60
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      thresholdRange:
         max: 500
      interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 1m -q 5 -c 2 http://podinfo.test"
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

kubectl -n test get services.serving podinfo -oyaml | grep 'flagger.app/primary-revision: podinfo-00001'

echo '✔ Canary initialization test passed'

echo '>>> Triggering canary deployment'
kubectl -n test patch services.serving podinfo --type=json -p '[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "ghcr.io/stefanprodan/podinfo:6.0.1"}]'

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get services.serving podinfo -oyaml | grep 'flagger.app/primary-revision: podinfo-00002' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        kubectl -n test get services.serving podinfo -oyaml
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

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: knative
  targetRef:
    apiVersion: serving.knative.dev/v1
    kind: Service
    name: podinfo
  progressDeadlineSeconds: 60
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      thresholdRange:
         max: 500
      interval: 1m
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 1m -q 5 -c 2 http://podinfo.test/delay/1"
          logCmdOutput: "true"
EOF

echo '>>> Triggering canary deployment'
kubectl -n test patch services.serving podinfo --type=json -p '[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "ghcr.io/stefanprodan/podinfo:6.0.2"}]'

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary rollback test passed'
