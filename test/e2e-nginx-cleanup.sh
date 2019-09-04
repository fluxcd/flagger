#!/usr/bin/env bash

set -o errexit

export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo '>>> Deleting NGINX Ingress'
helm delete --purge nginx-ingress

echo '>>> Deleting Flagger'
helm delete --purge flagger

echo '>>> Cleanup test namespace'
kubectl delete namespace test --ignore-not-found=true