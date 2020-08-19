#!/usr/bin/env bash

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Deleting Skipper Ingress'
kustomize build ${REPO_ROOT}/test/skipper | kubectl delete --force --wait=false -f -

echo '>>> Deleting Flagger'
kubectl delete namespace flagger-system --ignore-not-found=true --force --wait=false

echo '>>> Cleanup test namespace'
kubectl delete namespace test --ignore-not-found=true --force --wait=false

exit 0
