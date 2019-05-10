#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Building Flagger'
cd ${REPO_ROOT} && docker build -t test/flagger:latest . -f Dockerfile

echo '>>> Installing Flagger'
kind load docker-image test/flagger:latest
kubectl apply -f ${REPO_ROOT}/artifacts/flagger/

if [ -n "$1" ]; then
  kubectl -n istio-system set env deployment/flagger "MESH_PROVIDER=$1"
fi

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger
