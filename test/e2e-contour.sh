#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

CONTOUR_VER="v1.5.0"

echo '>>> Installing Contour'
kubectl apply -f https://projectcontour.io/quickstart/${CONTOUR_VER}/contour.yaml

kubectl -n projectcontour rollout status deployment/contour
kubectl -n projectcontour get all

echo '>>> Installing Flagger'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
--namespace projectcontour \
--set prometheus.install=true \
--set meshProvider=contour \
--set ingressClass=contour

kubectl -n projectcontour set image deployment/flagger flagger=test/flagger:latest

kubectl -n projectcontour rollout status deployment/flagger
kubectl -n projectcontour rollout status deployment/flagger-prometheus
