#!/usr/bin/env bash

set -o errexit

LINKERD_VER="stable-2.4.0"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

curl -SsL https://github.com/linkerd/linkerd2/releases/download/${LINKERD_VER}/linkerd2-cli-${LINKERD_VER}-linux > ${REPO_ROOT}/bin/linkerd
chmod +x ${REPO_ROOT}/bin/linkerd

echo ">>> Installing Linkerd ${LINKERD_VER}"
${REPO_ROOT}/bin/linkerd install | kubectl apply -f -
${REPO_ROOT}/bin/linkerd check

kubectl -n linkerd rollout status deployment/linkerd-controller
kubectl -n linkerd rollout status deployment/linkerd-proxy-injector

echo '>>> Load Flagger image in Kind'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace linkerd \
--set leaderElection.enabled=true \
--set leaderElection.replicaCount=2 \
--set metricsServer=http://linkerd-prometheus:9090 \
--set meshProvider=smi:linkerd

kubectl -n linkerd set image deployment/flagger flagger=test/flagger:latest
kubectl -n linkerd rollout status deployment/flagger