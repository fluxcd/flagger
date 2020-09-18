#!/usr/bin/env bash

# This script setups the scenarios for istio tests by creating a Kubernetes namespace, installing the load tester and a test workload (podinfo)
# Prerequisites: Kubernetes Kind and Istio

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Creating test namespace'
kubectl create namespace test
kubectl label namespace test istio-injection=enabled

echo '>>> Installing the load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Deploy podinfo'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml
