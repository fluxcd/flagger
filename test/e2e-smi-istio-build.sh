#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Building Flagger'
cd ${REPO_ROOT} && docker build -t test/flagger:latest . -f Dockerfile

kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--wait \
--namespace istio-system \
--set meshProvider=smi:istio

kubectl -n istio-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n istio-system rollout status deployment/flagger

echo '>>> Installing the SMI Istio adapter'
kubectl apply -f ${REPO_ROOT}/artifacts/smi/istio-adapter.yaml

kubectl -n istio-system rollout status deployment/smi-adapter-istio