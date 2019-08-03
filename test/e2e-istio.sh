#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.2.3"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo ">>> Installing Istio ${ISTIO_VER}"
helm repo add istio.io https://storage.googleapis.com/istio-release/releases/${ISTIO_VER}/charts

echo '>>> Installing Istio CRDs'
helm upgrade -i istio-init istio.io/istio-init --wait --namespace istio-system

echo '>>> Waiting for Istio CRDs to be ready'
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-10
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-11
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-12

echo '>>> Installing Istio control plane'
helm upgrade -i istio istio.io/istio --wait --namespace istio-system -f ${REPO_ROOT}/test/e2e-istio-values.yaml

kubectl -n istio-system get all

echo '>>> Load Flagger image in Kind'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/istio

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger