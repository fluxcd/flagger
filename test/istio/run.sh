#!/usr/bin/env bash

set -o errexit

DIR="$(cd "$(dirname "$0")" && pwd)"

"$DIR"/install.sh

"$DIR"/test-init.sh
"$DIR"/test-canary.sh

"$DIR"/test-init.sh
"$DIR"/test-skip-analysis.sh

"$DIR"/test-init.sh
"$DIR"/test-delegation.sh
