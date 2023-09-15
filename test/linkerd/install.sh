#!/usr/bin/env bash

set -o errexit

LINKERD_VER="stable-2.14.0"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

curl -SsL https://github.com/linkerd/linkerd2/releases/download/${LINKERD_VER}/linkerd2-cli-${LINKERD_VER}-linux-amd64 > ${REPO_ROOT}/bin/linkerd
chmod +x ${REPO_ROOT}/bin/linkerd
curl -SsL https://github.com/linkerd/linkerd-smi/releases/download/v${LINKERD_SMI_VER}/linkerd-smi-${LINKERD_SMI_VER}-linux-amd64 > ${REPO_ROOT}/bin/linkerd-smi
chmod +x ${REPO_ROOT}/bin/linkerd-smi

echo ">>> Installing Linkerd ${LINKERD_VER}"
${REPO_ROOT}/bin/linkerd install --crds | kubectl apply -f -
${REPO_ROOT}/bin/linkerd install | kubectl apply -f -
${REPO_ROOT}/bin/linkerd check

echo ">>> Installing Linkerd Viz"
${REPO_ROOT}/bin/linkerd viz install | kubectl apply -f -
kubectl -n linkerd-viz rollout status deploy/prometheus
${REPO_ROOT}/bin/linkerd viz check

# Scale down Deployments we don't need as they take up CPU and block other
# pods from being scheduled later.
kubectl -n linkerd-viz scale deploy web --replicas=0
kubectl -n linkerd-viz scale deploy tap --replicas=0
kubectl -n linkerd-viz scale deploy tap-injector --replicas=0
kubectl -n linkerd-viz scale deploy metrics-api --replicas=0
# Delete this APIService as it blocks the deletion of the test ns later
# (since we delete the linkerd-viz/tap Deployment which in turns makes the
# APIService unavailable due to missing Endpoints).
kubectl delete apiservices v1alpha1.tap.linkerd.io

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/linkerd

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n flagger-system rollout status deployment/flagger
