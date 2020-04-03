#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
NGINX_HELM_VERSION=1.34.3 # ingress v0.30.0

echo '>>> Installing NGINX Ingress'
kubectl create ns ingress-nginx
helm upgrade -i nginx-ingress stable/nginx-ingress --version=${NGINX_HELM_VERSION} \
--wait \
--namespace ingress-nginx \
--set controller.metrics.enabled=true \
--set controller.podAnnotations."prometheus\.io/scrape"=true \
--set controller.podAnnotations."prometheus\.io/port"=10254 \
--set controller.service.type=NodePort

kubectl -n ingress-nginx rollout status deployment/nginx-ingress-controller
kubectl -n ingress-nginx get all

echo '>>> Loading Flagger image'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--set crd.create=false \
--namespace ingress-nginx \
--set prometheus.install=true \
--set meshProvider=nginx

kubectl -n ingress-nginx set image deployment/flagger flagger=test/flagger:latest

kubectl -n ingress-nginx rollout status deployment/flagger
kubectl -n ingress-nginx rollout status deployment/flagger-prometheus

