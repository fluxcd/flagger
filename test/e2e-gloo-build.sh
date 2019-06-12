#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Building Flagger'
cd ${REPO_ROOT} && docker build -t test/flagger:latest . -f Dockerfile

echo '>>> Installing Flagger'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace gloo-system \
--set prometheus.install=true \
--set meshProvider=gloo

# Give flagger permissions for gloo objects
kubectl create clusterrolebinding flagger-gloo  --clusterrole=gloo-role-gateway --serviceaccount=gloo-system:flagger

kubectl -n gloo-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n gloo-system rollout status deployment/flagger
kubectl -n gloo-system rollout status deployment/flagger-prometheus
