#!/usr/bin/env bash

set -o errexit

KUMA_VER="2.7.5"
REPO_ROOT=$(git rev-parse --show-toplevel)
mkdir -p ${REPO_ROOT}/bin

echo ">>> Downloading Kuma ${KUMA_VER}"
curl -L https://docs.konghq.com/mesh/installer.sh | VERSION=${KUMA_VER} sh -
cp kong-mesh-${KUMA_VER}/bin/kumactl ${REPO_ROOT}/bin/kumactl
chmod +x ${REPO_ROOT}/bin/kumactl

echo ">>> Installing Kuma ${KUMA_VER}"
${REPO_ROOT}/bin/kumactl install control-plane | kubectl apply -f -

echo ">>> Waiting for Kuma Control Plane to be ready"
kubectl wait --for condition=established crd/meshes.kuma.io
kubectl -n kong-mesh-system rollout status deployment/kong-mesh-control-plane

echo ">>> Installing Prometheus"
${REPO_ROOT}/bin/kumactl install observability --components "prometheus" | kubectl apply -f -
kubectl -n mesh-observability rollout status deployment/prometheus-server

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/kuma

kubectl -n kong-mesh-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n kong-mesh-system rollout status deployment/flagger
