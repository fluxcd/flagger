#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

CONTOUR_VER="release-1.3"

echo '>>> Installing Contour'
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VER}/examples/render/contour.yaml

kubectl -n projectcontour rollout status deployment/contour
kubectl -n projectcontour get all

echo '>>> Installing Flagger'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace projectcontour \
--set prometheus.install=true \
--set meshProvider=contour

kubectl -n projectcontour set image deployment/flagger flagger=test/flagger:latest

kubectl -n projectcontour rollout status deployment/flagger
kubectl -n projectcontour rollout status deployment/flagger-prometheus
