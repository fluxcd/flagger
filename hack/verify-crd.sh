#!/usr/bin/env bash

REPO_ROOT=$(git rev-parse --show-toplevel)

d=$(diff ${REPO_ROOT}/artifacts/flagger/crd.yaml ${REPO_ROOT}/charts/flagger/crds/crd.yaml)
if [[ "$d" != "" ]]; then
    echo "⨯ ${REPO_ROOT}/artifacts/flagger/crd.yaml and ${REPO_ROOT}/charts/flagger/crds/crd.yaml don't match"
    echo "$d"
    exit 1
fi

d=$(diff ${REPO_ROOT}/artifacts/flagger/crd.yaml ${REPO_ROOT}/kustomize/base/flagger/crd.yaml)
if [[ "$d" != "" ]]; then
    echo "⨯ ${REPO_ROOT}/artifacts/flagger/crd.yaml and ${REPO_ROOT}/kustomize/base/flagger/crd.yaml don't match"
    echo "$d"
    exit 1
fi

echo "✔ CRDs verified"
