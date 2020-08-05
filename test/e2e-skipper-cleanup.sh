#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Deleting Skipper Ingress'
kustomize build test/skipper/ | kubectl delete -f -

echo '>>> Deleting Flagger'
kubectl delete -k ${REPO_ROOT}/kustomize/kubernetes

echo '>>> Cleanup test namespace'
kubectl delete namespace test --ignore-not-found=true
