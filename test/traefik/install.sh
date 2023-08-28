#!/usr/bin/env bash

set -o errexit

TRAEFIK_CHART_VERSION="24.0.0" # traefik 2.10.4
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Creating traefik namespace'
kubectl create ns traefik

echo '>>> Installing Traefik'
helm repo add traefik https://helm.traefik.io/traefik
cat <<EOF | helm upgrade -i traefik traefik/traefik --version=${TRAEFIK_CHART_VERSION} --wait --namespace traefik -f -
deployment:
  podAnnotations:
    prometheus.io/port: "9100"
    prometheus.io/scrape: "true"
    prometheus.io/path: "/metrics"
metrics:
  prometheus:
    entryPoint: metrics
service:
  enabled: true
  type: NodePort
EOF

kubectl -n traefik rollout status deployment/traefik
kubectl -n traefik get all
kubectl -n traefik get deployment/traefik -oyaml
kubectl -n traefik get service/traefik -oyaml

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--set crd.create=false \
--namespace traefik \
--set prometheus.install=true \
--set meshProvider=traefik \
--set image.repository=test\/flagger \
--set image.tag=latest \

kubectl -n traefik rollout status deployment/flagger
kubectl -n traefik rollout status deployment/flagger-prometheus
