#!/usr/bin/env bash

set -o errexit

GLOO_VER="1.14.10"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing Gloo'
kubectl create ns gloo-system
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm upgrade -i gloo gloo/gloo --version ${GLOO_VER} \
--namespace gloo-system \
--set discovery.enabled=false

kubectl -n gloo-system rollout status deployment/gloo
kubectl -n gloo-system get all

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--set crd.create=false \
--namespace gloo-system \
--set prometheus.install=true \
--set meshProvider=gloo

kubectl -n gloo-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n gloo-system rollout status deployment/flagger
kubectl -n gloo-system rollout status deployment/flagger-prometheus
