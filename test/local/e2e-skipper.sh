#!/usr/bin/env bash

# This script is intended for local workstation development convenience.
# It will run the e2e tests for Skipper and leave a working setup to play with

REPO_ROOT=$(git rev-parse --show-toplevel)
cd $REPO_ROOT

make test
make build
docker tag weaveworks/flagger:latest test/flagger:latest
make loadtester-build
(kind get clusters && kubectl delete ns/test --force) || kind create cluster --wait 5m --image kindest/node:v1.16.9
./test/e2e-skipper.sh
# port forward prometheus UI to localhost:9090
kubectl port-forward $(kubectl get pods -l=app=flagger-prometheus -o name -n flagger-system | head -n 1) 9090:9090 -n flagger-system &

./test/e2e-skipper-tests.sh
