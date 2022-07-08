#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace=gloo-mesh \
--set crd.create=false \
--set meshProvider=gloo-mesh \
--set metricsServer=http://prometheus-server

kubectl -n gloo-mesh set image deployment/flagger flagger=test/flagger:latest

kubectl -n gloo-mesh rollout status deployment/flagger
