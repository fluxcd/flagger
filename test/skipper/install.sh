#!/usr/bin/env bash

set -o errexit

KUSTOMIZE_VERSION=3.8.2
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing Kustomize'
cd ${REPO_ROOT}/bin && kustomize_url=https://github.com/kubernetes-sigs/kustomize/releases/download && \
curl -sL ${kustomize_url}/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_amd64.tar.gz | \
tar xz

echo '>>> Installing Skipper'
${REPO_ROOT}/bin/kustomize build ${REPO_ROOT}/test/skipper | kubectl apply -f -

kubectl -n kube-system rollout status deployment/skipper-ingress

echo '>>> Installing Flagger'
kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
kubectl -n flagger-system rollout status deployment/flagger-prometheus