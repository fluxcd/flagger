#!/usr/bin/env bash

set -o errexit
set -o pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
CODEGEN_VERSION="@v0.0.0-20190620073620-d55040311883"
CODEGEN_PKG=${CODEGEN_PKG:-$(echo `go env GOPATH`"/pkg/mod/k8s.io/code-generator${CODEGEN_VERSION}")}

if [[ "$CIRCLECI" ]]; then
    ls $(echo `go env GOPATH`'/pkg/mod/k8s.io/');
    mkdir -p ${REPO_ROOT}/bin;
    cp -r ${CODEGEN_PKG} ${REPO_ROOT}/bin/;
    CODEGEN_PKG=${REPO_ROOT}/bin/code-generator${CODEGEN_VERSION};
    echo ">> $CODEGEN_PKG";
fi

chmod +x ${CODEGEN_PKG}/generate-groups.sh

${CODEGEN_PKG}/generate-groups.sh all \
    github.com/weaveworks/flagger/pkg/client github.com/weaveworks/flagger/pkg/apis \
    "appmesh:v1beta1 istio:v1alpha3 flagger:v1alpha3 smi:v1alpha1" \
    --go-header-file ${REPO_ROOT}/hack/boilerplate.go.txt
