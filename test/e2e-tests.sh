#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Building Flagger'
cd ${REPO_ROOT} && docker build -t test/flagger:latest . -f Dockerfile

echo '>>> Installing Flagger'
kind load docker-image test/flagger:latest
kubectl apply -f ${REPO_ROOT}/artifacts/flagger/
kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger

echo '>>> Creating test namespace'
kubectl create namespace test
kubectl label namespace test istio-injection=enabled

echo '>>> Installing the load tester'
kubectl -n test apply -f ${REPO_ROOT}/artifacts/loadtester/
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml

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
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: istio_requests_total
      threshold: 99
      interval: 1m
    - name: istio_request_duration_seconds_bucket
      threshold: 500
      interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 1m -q 10 -c 2 http://podinfo.test:9898/"

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

echo '>>> Canary initialization test passed ✔︎'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=quay.io/stefanprodan/podinfo:1.4.2

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '1.4.2' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Canary promotion test passed ✔︎'

kubectl -n istio-system logs deployment/flagger

echo '>>> All tests passed ✔︎'