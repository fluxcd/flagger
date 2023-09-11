#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
DIR="$(cd "$(dirname "$0")" && pwd)"

"$DIR"/install.sh

"$REPO_ROOT"/test/workloads/init.sh
"$DIR"/test-canary.sh
"$DIR"/test-bg.sh
"$DIR"/test-ab.sh
"$DIR"/test-session-affinity.sh
