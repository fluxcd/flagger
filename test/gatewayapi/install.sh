#!/usr/bin/env bash

set -o errexit

CONTOUR_VER="release-1.20"
GATEWAY_API_VER="v1alpha2"
REPO_ROOT=$(git rev-parse --show-toplevel)
KUSTOMIZE_VERSION=4.5.2
OS=$(uname -s)
ARCH=$(arch)

mkdir -p ${REPO_ROOT}/bin

echo ">>> Installing Contour ${CONTOUR_VER}, Gateway API components ${GATEWAY_API_VER}"
# retry if it fails, creating a gateway object is flaky sometimes
until cd ${REPO_ROOT}/bin && kubectl apply -f \
    https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VER}/examples/render/contour-gateway.yaml; do
    sleep 1
done

kubectl -n projectcontour rollout status deployment/contour
kubectl -n projectcontour get all

echo '>>> Installing Kustomize'
cd ${REPO_ROOT}/bin && kustomize_url=https://github.com/kubernetes-sigs/kustomize/releases/download && \
curl -sL ${kustomize_url}/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_${OS}_${ARCH}.tar.gz | \
tar xz

echo '>>> Installing Flagger'
${REPO_ROOT}/bin/kustomize build ${REPO_ROOT}/test/gatewayapi | kubectl apply -f -

kubectl -n projectcontour set image deployment/flagger flagger=test/flagger:latest
kubectl -n projectcontour rollout status deployment/flagger
