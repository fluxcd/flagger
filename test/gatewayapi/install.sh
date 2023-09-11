#!/usr/bin/env bash

set -o errexit

CONTOUR_VER="v1.26.0"
GATEWAY_API_VER="v1beta1"
REPO_ROOT=$(git rev-parse --show-toplevel)
KUSTOMIZE_VERSION=4.5.2
OS=$(uname -s)
ARCH=$(arch)
if [[ $ARCH == "x86_64" ]]; then
    ARCH="amd64"
fi

mkdir -p ${REPO_ROOT}/bin

echo ">>> Installing Contour components, Gateway API CRDs"
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/${CONTOUR_VER}/examples/render/contour-gateway-provisioner.yaml

kubectl -n projectcontour rollout status deployment/contour-gateway-provisioner
kubectl -n gateway-system wait --for=condition=complete job/gateway-api-admission
kubectl -n gateway-system wait --for=condition=complete job/gateway-api-admission-patch
kubectl -n gateway-system rollout status deployment/gateway-api-admission-server
kubectl -n projectcontour get all

echo ">>> Creating GatewayClass"
cat <<EOF | kubectl apply -f -
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
EOF

echo ">>> Creating Gateway"
cat <<EOF | kubectl apply -f -
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
  namespace: projectcontour
spec:
  gatewayClassName: contour
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
EOF

echo '>>> Installing Kustomize'
cd ${REPO_ROOT}/bin && \
    curl -sL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_${OS}_${ARCH}.tar.gz | \
    tar xz

echo '>>> Installing Flagger'
${REPO_ROOT}/bin/kustomize build ${REPO_ROOT}/kustomize/gatewayapi | kubectl apply -f -
kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest

kubectl -n flagger-system rollout status deployment/flagger
