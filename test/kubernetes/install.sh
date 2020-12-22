#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/kubernetes

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
kubectl -n flagger-system rollout status deployment/flagger-prometheus
