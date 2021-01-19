#!/usr/bin/env bash

# This script creates the test app and load tester

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Delete test namespace'
kubectl delete namespace test --ignore-not-found=true --wait=true

echo '>>> Creating test namespace'
kubectl create namespace test
kubectl label namespace test istio-injection=enabled
kubectl annotate namespace test linkerd.io/inject=enabled

echo '>>> Installing the load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Deploy podinfo'
kubectl apply -f ${REPO_ROOT}/test/workloads/secret.yaml
kubectl apply -f ${REPO_ROOT}/test/workloads/deployment.yaml
kubectl apply -f ${REPO_ROOT}/test/workloads/daemonset.yaml
