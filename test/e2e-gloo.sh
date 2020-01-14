#!/usr/bin/env bash

set -o errexit

GLOO_VER="0.20.2"
REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Installing Gloo'
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm upgrade -i gloo gloo/gloo --version ${GLOO_VER} \
--namespace gloo-system \
--set discovery.enabled=true

kubectl -n gloo-system rollout status deployment/gloo
kubectl -n gloo-system rollout status deployment/gateway-proxy-v2
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