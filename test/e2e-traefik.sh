#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
TRAEFIK_CHART_VERSION="9.11.0" # traefik 2.3.3

echo '>>> Creating traefik namespace'
kubectl create ns traefik

echo '>>> Installing Traefik'
# helm repo add traefik https://helm.traefik.io/traefik
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

echo '>>> Loading Flagger image'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--set crd.create=false \
--namespace traefik \
--set prometheus.install=true \
--set meshProvider=traefik \
--set image.repository=test\/flagger \
--set image.tag=latest \

kubectl -n traefik rollout status deployment/flagger
