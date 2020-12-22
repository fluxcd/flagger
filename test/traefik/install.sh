#!/usr/bin/env bash

set -o errexit

TRAEFIK_CHART_VERSION="9.11.0" # traefik 2.3.3
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Creating traefik namespace'
kubectl create ns traefik

echo '>>> Installing Traefik'
helm repo add traefik https://helm.traefik.io/traefik
cat <<EOF | helm upgrade -i traefik traefik/traefik --version=${TRAEFIK_VERSION} --namespace traefik -f -
additionalArguments:
  - "--metrics.prometheus=true"
deployment:
  podAnnotations:
    "prometheus.io/port": "9000"
    "prometheus.io/scrape": "true"
EOF

kubectl -n traefik rollout status deployment/traefik
kubectl -n traefik get all

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
