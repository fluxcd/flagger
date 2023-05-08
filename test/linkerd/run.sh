#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
DIR="$(cd "$(dirname "$0")" && pwd)"

"$DIR"/install.sh

"$REPO_ROOT"/test/workloads/init.sh
# Delete Daemonset as it eats up precious CPU requests and we don't need it anyway.
kubectl -n test delete ds podinfo-ds
"$DIR"/test-canary.sh

"$REPO_ROOT"/test/workloads/init.sh
# Delete Daemonset as it eats up precious CPU requests and we don't need it anyway.
kubectl -n test delete ds podinfo-ds
"$DIR"/test-steps.sh
