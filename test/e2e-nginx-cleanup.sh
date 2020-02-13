#!/usr/bin/env bash

set -o errexit

echo '>>> Deleting NGINX Ingress'
helm delete nginx-ingress -n ingress-nginx

echo '>>> Deleting Flagger'
helm delete flagger -n ingress-nginx

echo '>>> Cleanup test namespace'
kubectl delete namespace test --ignore-not-found=true