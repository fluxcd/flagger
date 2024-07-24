#!/usr/bin/env bash

set -o errexit

GATEWAY_API_VER="v1.0.0"
REPO_ROOT=$(git rev-parse --show-toplevel)
ISTIO_VER="1.20.0"

mkdir -p ${REPO_ROOT}/bin

echo ">>> Installing Gateway API CRDs"
kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null || \
  { kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=${GATEWAY_API_VER}" | kubectl apply -f -; }

echo ">>> Downloading Istio ${ISTIO_VER}"
cd ${REPO_ROOT}/bin && \
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VER} sh -

echo ">>> Installing Istio ${ISTIO_VER}"
${REPO_ROOT}/bin/istio-${ISTIO_VER}/bin/istioctl install --set profile=minimal \
  --set values.pilot.resources.requests.cpu=100m \
  --set values.pilot.resources.requests.memory=100Mi -y

kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/prometheus.yaml
kubectl -n istio-system rollout status deployment/prometheus

echo ">>> Creating Gateway"
kubectl create ns istio-ingress
cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: gateway
  namespace: istio-ingress
spec:
  gatewayClassName: istio
  listeners:
  - name: default
    hostname: "*.example.com"
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
EOF

echo '>>> Installing Flagger'
kubectl create ns flagger-system
helm upgrade -i flagger ${REPO_ROOT}/charts/flagger \
  --set crd.create=false \
  --namespace flagger-system \
  --set prometheus.install=false \
  --set meshProvider=gatewayapi:v1 \
  --set metricsServer=http://prometheus.istio-system:9090

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n flagger-system rollout status deployment/flagger
