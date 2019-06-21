#!/usr/bin/env bash

set -o errexit

cp /tmp/bin/flagger . && chmod +x flagger
docker build -t test/flagger:latest . -f ./test/Dockerfile.ci
