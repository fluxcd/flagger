#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.5.2"
REPO_ROOT=$(git rev-parse --show-toplevel)

echo ">>> Downloading Istio ${ISTIO_VER}"
cd ${REPO_ROOT}/bin && \
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VER} sh -

echo ">>> Installing Istio ${ISTIO_VER}"
${REPO_ROOT}/bin/istio-${ISTIO_VER}/bin/istioctl manifest apply --set profile=default

kubectl -n istio-system rollout status deployment/prometheus

kubectl -n istio-system get all

echo '>>> Load Flagger image in Kind'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/istio

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger
