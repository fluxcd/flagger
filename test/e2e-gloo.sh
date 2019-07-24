#!/usr/bin/env bash

set -o errexit

GLOO_VER="0.17.5"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Installing Gloo'
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm upgrade -i gloo gloo/gloo --version ${GLOO_VER} \
--wait \
--namespace gloo-system \
--set gatewayProxies.gateway-proxy.service.type=NodePort

kubectl -n gloo-system rollout status deployment/gloo
kubectl -n gloo-system rollout status deployment/gateway-proxy
kubectl -n gloo-system get all

echo '>>> Installing Flagger'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace gloo-system \
--set prometheus.install=true \
--set meshProvider=gloo

kubectl -n gloo-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n gloo-system rollout status deployment/flagger
kubectl -n gloo-system rollout status deployment/flagger-prometheus