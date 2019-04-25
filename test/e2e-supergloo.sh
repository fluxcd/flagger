#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.0.6"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo ">>> Downloading Supergloo CLI"
curl -SsL https://github.com/solo-io/supergloo/releases/download/v0.3.13/supergloo-cli-linux-amd64 > supergloo-cli
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

# add rbac rules
kubectl create clusterrolebinding flagger-supergloo --clusterrole=mesh-discovery --serviceaccount=istio-system:flagger

kubectl -n istio-system rollout status deployment/istio-pilot
kubectl -n istio-system rollout status deployment/istio-policy
kubectl -n istio-system rollout status deployment/istio-sidecar-injector
kubectl -n istio-system rollout status deployment/istio-telemetry
kubectl -n istio-system rollout status deployment/prometheus

kubectl -n istio-system get all
