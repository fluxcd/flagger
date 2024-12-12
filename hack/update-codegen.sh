#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(git rev-parse --show-toplevel)

# Grab code-generator version from go.sum.
CODEGEN_VERSION=$(grep 'k8s.io/code-generator' go.sum | awk '{print $2}' | tail -1 | awk -F '/' '{print $1}')
CODEGEN_PKG=$(echo `go env GOPATH`"/pkg/mod/k8s.io/code-generator@${CODEGEN_VERSION}")

echo ">> Using ${CODEGEN_PKG}"

# code-generator does work with go.mod but makes assumptions about
# the project living in `$GOPATH/src`. To work around this and support
# any location; create a temporary directory, use this as an output
# base, and copy everything back once generated.
TEMP_DIR=$(mktemp -d)
cleanup() {
    echo ">> Removing ${TEMP_DIR}"
    rm -rf ${TEMP_DIR}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output directory ${TEMP_DIR}"

PACKAGE_PATH_BASE="github.com/fluxcd/flagger"

mkdir -p "${TEMP_DIR}/${PACKAGE_PATH_BASE}/pkg/client/informers" \
         "${TEMP_DIR}/${PACKAGE_PATH_BASE}/pkg/client/listers" \
         "${TEMP_DIR}/${PACKAGE_PATH_BASE}/pkg/client/clientset"

# Ensure we can execute.
chmod +x ${CODEGEN_PKG}/kube_codegen.sh

source ${CODEGEN_PKG}/kube_codegen.sh

kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    ./pkg/apis

kube::codegen::gen_client \
    --output-dir "${TEMP_DIR}/${PACKAGE_PATH_BASE}/pkg/client" \
    --output-pkg "${PACKAGE_PATH_BASE}/pkg/client" \
    --with-watch \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    ./pkg/apis

tree $TEMP_DIR/${PACKAGE_PATH_BASE/pkg/client}/

# Copy everything back.
cp -r "${TEMP_DIR}/${PACKAGE_PATH_BASE}/." "${SCRIPT_ROOT}/"
