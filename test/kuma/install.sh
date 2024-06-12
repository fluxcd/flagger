#!/usr/bin/env bash

set -o errexit

KUMA_VER="2.2.5"
REPO_ROOT=$(git rev-parse --show-toplevel)
mkdir -p ${REPO_ROOT}/bin

echo ">>> Downloading Kuma ${KUMA_VER}"
curl -SsL \
    https://packages.konghq.com/public/kuma-binaries-release/raw/names/kuma-linux-amd64/versions/${KUMA_VER}/kuma-${KUMA_VER}-linux-amd64.tar.gz \
    -o kuma-${KUMA_VER}.tar.gz
tar xvzf kuma-${KUMA_VER}.tar.gz
cp kuma-${KUMA_VER}/bin/kumactl ${REPO_ROOT}/bin/kumactl
chmod +x ${REPO_ROOT}/bin/kumactl

echo ">>> Installing Kuma ${KUMA_VER}"
${REPO_ROOT}/bin/kumactl install control-plane | kubectl apply -f -

echo ">>> Waiting for Kuma Control Plane to be ready"
kubectl wait --for condition=established crd/meshes.kuma.io
kubectl -n kuma-system rollout status deployment/kuma-control-plane

echo ">>> Installing Prometheus"
${REPO_ROOT}/bin/kumactl install observability --components "prometheus" | kubectl apply -f -
kubectl -n mesh-observability rollout status deployment/prometheus-server

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/kuma

kubectl -n kuma-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n kuma-system rollout status deployment/flagger
