#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.18.2"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo ">>> Downloading Istio ${ISTIO_VER}"
cd ${REPO_ROOT}/bin && \
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VER} sh -

echo ">>> Installing Istio ${ISTIO_VER}"
${REPO_ROOT}/bin/istio-${ISTIO_VER}/bin/istioctl manifest install --set profile=default --skip-confirmation \
  --set values.pilot.resources.requests.cpu=100m \
  --set values.pilot.resources.requests.memory=100Mi

kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.18/samples/addons/prometheus.yaml
kubectl -n istio-system rollout status deployment/prometheus

kubectl -n istio-system get all

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/istio

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger
