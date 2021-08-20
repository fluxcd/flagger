#!/usr/bin/env bash

set -o errexit

LINKERD_VER="stable-2.10.2"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

curl -SsL https://github.com/linkerd/linkerd2/releases/download/${LINKERD_VER}/linkerd2-cli-${LINKERD_VER}-linux-amd64 > ${REPO_ROOT}/bin/linkerd
chmod +x ${REPO_ROOT}/bin/linkerd

echo ">>> Installing Linkerd ${LINKERD_VER}"
${REPO_ROOT}/bin/linkerd install | kubectl apply -f -
${REPO_ROOT}/bin/linkerd check

echo ">>> Installing Linkerd Viz"
${REPO_ROOT}/bin/linkerd viz install | kubectl apply -f -
${REPO_ROOT}/bin/linkerd viz check

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/linkerd

kubectl -n linkerd set image deployment/flagger flagger=test/flagger:latest
kubectl -n linkerd rollout status deployment/flagger
