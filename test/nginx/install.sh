#!/usr/bin/env bash

set -o errexit

NGINX_HELM_VERSION=3.36.0 # ingress v0.49.0
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing NGINX Ingress'
kubectl create ns ingress-nginx
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm upgrade -i ingress-nginx ingress-nginx/ingress-nginx --version=${NGINX_HELM_VERSION} \
--wait \
--namespace ingress-nginx \
--set controller.metrics.enabled=true \
--set controller.admissionWebhooks.enabled=true \
--set controller.podAnnotations."prometheus\.io/scrape"=true \
--set controller.podAnnotations."prometheus\.io/port"=10254 \
--set controller.service.type=NodePort

kubectl -n ingress-nginx get all

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--set crd.create=false \
--namespace ingress-nginx \
--set prometheus.install=true \
--set meshProvider=nginx

kubectl -n ingress-nginx set image deployment/flagger flagger=test/flagger:latest

kubectl -n ingress-nginx rollout status deployment/flagger
kubectl -n ingress-nginx rollout status deployment/flagger-prometheus
