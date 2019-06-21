#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Load Flagger image in Kind'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace istio-system \
--set meshProvider=istio

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n istio-system rollout status deployment/flagger
