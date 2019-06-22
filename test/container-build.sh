#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin
cp /tmp/bin/flagger ${REPO_ROOT}/bin && chmod +x ${REPO_ROOT}/bin/flagger
cp /tmp/bin/loadtester ${REPO_ROOT}/bin && chmod +x ${REPO_ROOT}/bin/loadtester

docker build -t test/flagger:latest . -f ${REPO_ROOT}/Dockerfile
docker build -t test/flagger-loadtester:latest . -f ${REPO_ROOT}/Dockerfile.loadtester