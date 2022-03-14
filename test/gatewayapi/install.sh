#!/usr/bin/env bash

set -o errexit

CONTOUR_VER="release-1.20"
GATEWAY_API_VER="v1alpha2"
REPO_ROOT=$(git rev-parse --show-toplevel)
KUSTOMIZE_VERSION=4.5.2
OS=$(uname -s)
ARCH=$(arch)
if [[ $ARCH == "x86_64" ]]; then
    ARCH="amd64"
fi

mkdir -p ${REPO_ROOT}/bin

echo ">>> Installing Gateway API CRDs"
kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.1" \
| kubectl apply -f -

echo ">>> Installing Contour components, GatewayClass and Gateway"
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VER}/examples/render/contour-gateway.yaml

kubectl -n projectcontour rollout status deployment/contour
kubectl -n projectcontour get all
kubectl get gatewayclass -oyaml
kubectl -n projectcontour get gateway -oyaml

echo '>>> Installing Kustomize'
cd ${REPO_ROOT}/bin && \
    curl -sL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_${OS}_${ARCH}.tar.gz | \
    tar xz

echo '>>> Installing Flagger'
${REPO_ROOT}/bin/kustomize build ${REPO_ROOT}/kustomize/gatewayapi | kubectl apply -f -
kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
