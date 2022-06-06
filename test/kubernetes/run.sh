#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
DIR="$(cd "$(dirname "$0")" && pwd)"

"$DIR"/install.sh

"$REPO_ROOT"/test/workloads/init.sh
"$DIR"/test-deployment.sh
"$DIR"/test-daemonset.sh

kubectl -n test delete deploy podinfo
kubectl -n test delete svc podinfo-svc
kubectl apply -f ${REPO_ROOT}/test/workloads/deployment.yaml -n test
"$DIR"/test-hpa.sh
