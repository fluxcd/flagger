#!/usr/bin/env bash

set -o errexit

APISIX_CHART_VERSION="0.11.3" # apisix 2.15.1
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Creating apisix namespace'
kubectl create ns apisix

echo '>>> Installing APISIX'
helm repo add apisix https://charts.apiseven.com

helm upgrade -i apisix apisix/apisix --version=${APISIX_CHART_VERSION} \
--namespace apisix \
--set apisix.podAnnotations."prometheus\.io/scrape"=true \
--set apisix.podAnnotations."prometheus\.io/port"=9091 \
--set apisix.podAnnotations."prometheus\.io/path"=/apisix/prometheus/metrics \
--set pluginAttrs.prometheus.export_addr.ip=0.0.0.0 \
--set pluginAttrs.prometheus.export_addr.port=9091 \
--set pluginAttrs.prometheus.export_uri=/apisix/prometheus/metrics \
--set pluginAttrs.prometheus.metric_prefix=apisix_ \
--set ingress-controller.enabled=true \
--set ingress-controller.config.apisix.serviceNamespace=apisix

kubectl -n apisix rollout status deployment/apisix
kubectl -n apisix get all

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--set crd.create=false \
--namespace apisix \
--set prometheus.install=true \
--set meshProvider=apisix \
--set image.repository=test\/flagger \
--set image.tag=latest \

kubectl -n apisix get all
