#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.1.6"
SUPERGLOO_VER="v0.3.23"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo ">>> Downloading Supergloo CLI"
curl -SsL https://github.com/solo-io/supergloo/releases/download/${SUPERGLOO_VER}/supergloo-cli-linux-amd64 > supergloo-cli
chmod +x supergloo-cli

echo ">>> Installing Supergloo"
./supergloo-cli init
echo ">>> Installing Istio ${ISTIO_VER}"
kubectl create ns istio-system
./supergloo-cli install istio --name test --namespace supergloo-system --auto-inject=true --installation-namespace istio-system --mtls=false --prometheus=true --version ${ISTIO_VER}

echo '>>> Waiting for Istio to be ready'
until kubectl -n supergloo-system get mesh test
do
  sleep 2
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