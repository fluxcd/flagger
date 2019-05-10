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
--wait \
--namespace ingress-nginx \
--set prometheus.install=true \
--set meshProvider=nginx

kubectl -n ingress-nginx set image deployment/flagger flagger=test/flagger:latest

kubectl -n ingress-nginx rollout status deployment/flagger
kubectl -n ingress-nginx rollout status deployment/flagger-prometheus
