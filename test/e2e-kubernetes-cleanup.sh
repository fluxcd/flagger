#!/usr/bin/env bash

set -o errexit

echo '>>> Delete test namespace'
kubectl delete namespace test --ignore-not-found=true --wait=true
