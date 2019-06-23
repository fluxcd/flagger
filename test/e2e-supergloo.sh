#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.0.6"
SUPERGLOO_VER="v0.3.13"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo ">>> Downloading Supergloo CLI"
curl -SsL https://github.com/solo-io/supergloo/releases/download/${SUPERGLOO_VER}/supergloo-cli-linux-amd64 > ${REPO_ROOT}/bin/supergloo-cli
chmod +x ${REPO_ROOT}/bin/supergloo-cli

echo ">>> Installing Supergloo"
${REPO_ROOT}/bin/supergloo-cli init

echo ">>> Installing Istio ${ISTIO_VER}"
kubectl create ns istio-system
${REPO_ROOT}/bin/supergloo-cli install istio --name test --version ${ISTIO_VER} \
    --namespace supergloo-system --installation-namespace istio-system \
    --auto-inject=true --mtls=false --prometheus=true

echo '>>> Waiting for Istio to be ready'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n supergloo-system get mesh test && ok=true || ok=false
    sleep 10
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        echo "No more retries left"
        exit 1
    fi
done

kubectl -n istio-system rollout status deployment/istio-pilot
kubectl -n istio-system rollout status deployment/istio-policy
kubectl -n istio-system rollout status deployment/istio-sidecar-injector
kubectl -n istio-system rollout status deployment/istio-telemetry
kubectl -n istio-system rollout status deployment/prometheus

kubectl -n istio-system get all

echo '>>> Load Flagger image in Kind'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace istio-system \
--set meshProvider=supergloo:test.supergloo-system

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger

echo '>>> Adding Flagger Supergloo RBAC'
kubectl create clusterrolebinding flagger-supergloo --clusterrole=mesh-discovery --serviceaccount=istio-system:flagger
