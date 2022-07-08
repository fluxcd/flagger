#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing KEDA'
helm repo add kedacore https://kedacore.github.io/charts
kubectl create ns keda
helm install keda kedacore/keda --namespace keda --wait

kubectl -n keda get all

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/kubernetes

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
kubectl -n flagger-system rollout status deployment/flagger-prometheus
