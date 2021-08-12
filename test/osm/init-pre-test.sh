#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

${REPO_ROOT}/bin/linux-amd64/osm namespace add test
${REPO_ROOT}/bin/linux-amd64/osm metrics enable --namespace test

kubectl patch deployment flagger-loadtester -n test -p '{"spec": {"template": {"metadata": {"annotations": {"openservicemesh.io/inbound-port-exclusion-list": "80, 8080"}}}}}' --type=merge
kubectl -n test rollout restart deploy/flagger-loadtester
kubectl -n test rollout status deploy/flagger-loadtester
