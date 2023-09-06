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

# Ensure we can execute.
chmod +x ${CODEGEN_PKG}/generate-groups.sh
chmod +x ${CODEGEN_PKG}/generate-internal-groups.sh

${CODEGEN_PKG}/generate-groups.sh client,deepcopy,informer,lister \
    github.com/fluxcd/flagger/pkg/client github.com/fluxcd/flagger/pkg/apis \
    "flagger:v1beta1 appmesh:v1beta2 appmesh:v1beta1 istio:v1alpha3 smi:v1alpha1 smi:v1alpha2 smi:v1alpha3 gloo/gloo:v1 gloo/gateway:v1 projectcontour:v1 traefik:v1alpha1 kuma:v1alpha1 gatewayapi:v1alpha2 gatewayapi:v1beta1 keda:v1alpha1 apisix:v2" \
    --output-base "${TEMP_DIR}" \
    --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt

# Copy everything back.
cp -r "${TEMP_DIR}/github.com/fluxcd/flagger/." "${SCRIPT_ROOT}/"
