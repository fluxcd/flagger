#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Loading Flagger image'
kind load docker-image test/flagger:latest

echo '>>> Installing Skipper Ingress, Flagger and Prometheus'
# use kustomize to avoid compatibility issues:
# https://github.com/kubernetes-sigs/kustomize/issues/2390
# Skipper will throw an Prometheus warning which can be ignored:
# https://github.com/weaveworks/flagger/issues/664
kustomize build ${REPO_ROOT}/test/skipper | kubectl apply -f -

kubectl rollout status deployment/skipper-ingress -n kube-system
kubectl rollout status deployment/flagger-prometheus -n flagger-system

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
