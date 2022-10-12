#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
DIR="$(cd "$(dirname "$0")" && pwd)"

"$DIR"/create-kind-cluster.sh 1 cluster1
"$DIR"/install-gloomesh.sh
"$DIR"/install-flagger.sh

"$REPO_ROOT"/test/workloads/init.sh
"$DIR"/test-canary.sh
