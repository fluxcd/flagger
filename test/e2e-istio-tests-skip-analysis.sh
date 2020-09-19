#!/usr/bin/env bash

# This script runs e2e tests for when the canary analysis is skipped
# Prerequisites: Kubernetes Kind and Istio

set -o errexit

echo '>>> Initialising canary'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
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
    portDiscovery: true
  skipAnalysis: true
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo.test:9898/"
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
        kubectl -n istio-system logs deployment/flagger
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
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
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
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

if [[ "$1" = "canary" ]]; then
  exit 0
fi

echo '>>> Triggering canary deployment with a bad release (non existent docker image)'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/potato:1.0.0

echo '>>> Waiting for canary to fail'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl get canary/podinfo -n test -o=jsonpath='{.status.phase}' | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Confirm primary pod is still running and with correct version'
retries=50
count=0
ok=false
until ${okImage} && ${okRunning}; do
    kubectl get deployment podinfo-primary -n test -o jsonpath='{.spec.replicas}' | grep 1 && okRunning=true || okRunning=false
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.3' && okImage=true || okImage=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

kubectl -n istio-system logs deployment/flagger

echo '✔ All tests passed'
