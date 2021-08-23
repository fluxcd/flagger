#!/usr/bin/env bash

set -o errexit

CONTOUR_VER="release-1.18"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing Contour'
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VER}/examples/render/contour.yaml

kubectl -n projectcontour rollout status deployment/contour
kubectl -n projectcontour get all

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace projectcontour \
--set prometheus.install=true \
--set meshProvider=contour \
--set ingressClass=contour

kubectl -n projectcontour set image deployment/flagger flagger=test/flagger:latest

kubectl -n projectcontour rollout status deployment/flagger
kubectl -n projectcontour rollout status deployment/flagger-prometheus
