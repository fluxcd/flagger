#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Installing Skipper Ingress'
kustomize build test/skipper/ | kubectl apply -f -
kubectl rollout status deployment/skipper-ingress -n kube-system
kubectl get all --all-namespaces

echo '>>> Loading Flagger image'
kind load docker-image test/flagger:latest

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/kubernetes

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
kubectl -n flagger-system rollout status deployment/flagger-prometheus
