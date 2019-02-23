#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.1.0-rc.0"
REPO_ROOT=$(git rev-parse --show-toplevel)

echo ">>> Starting e2e testing using Istio ${ISTIO_VER}"
kind create cluster --wait 5m

export KUBECONFIG="$(kind get kubeconfig-path)"
kubectl version

echo '>>> Installing Tiller'
kubectl --namespace kube-system create sa tiller
kubectl create clusterrolebinding tiller-cluster-rule --clusterrole=cluster-admin --serviceaccount=kube-system:tiller
helm init --service-account tiller --upgrade --wait

helm repo add istio.io https://storage.googleapis.com/istio-release/releases/${ISTIO_VER}/charts

echo '>>> Installing Istio CRDs'
helm upgrade -i istio-init istio.io/istio-init --wait --namespace istio-system

echo '>>> Waiting for Istio CRDs to be ready'
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-10
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-11

echo 'Installing Istio control plane'
helm upgrade -i istio istio.io/istio --wait --namespace istio-system -f ${REPO_ROOT}/e2e/istio-values.yaml

export KUBECONFIG="$(kind get kubeconfig-path)"

echo '>>> Installing Flagger'
#cd ${REPO_ROOT} && docker build -t stefanprodan/flagger:latest . -f Dockerfile
#kind load docker-image stefanprodan/flagger:latest
kubectl apply -f ${REPO_ROOT}/artifacts/flagger/
#kubectl -n istio-system set image deployment/flagger flagger=stefanprodan/flagger:latest
kubectl -n istio-system rollout status deployment/flagger

echo '>>> Creating test namespace'
kubectl create namespace test
kubectl label namespace test istio-injection=enabled

echo '>>> Installing the load tester'
kubectl -n test apply -f ${REPO_ROOT}/artifacts/loadtester/
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/e2e/workload.yaml

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
    interval: 10s
    threshold: 10
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
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Canary promotion test passed ✔︎'
