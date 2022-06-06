#!/usr/bin/env bash

set -o errexit

KUMA_VER="1.6.0"
REPO_ROOT=$(git rev-parse --show-toplevel)
KUSTOMIZE_VERSION=3.8.2
mkdir -p ${REPO_ROOT}/bin

echo ">>> Downloading Kuma ${KUMA_VER}"
curl -SsL https://download.konghq.com/mesh-alpine/kuma-${KUMA_VER}-ubuntu-amd64.tar.gz -o kuma-${KUMA_VER}.tar.gz
tar xvzf kuma-${KUMA_VER}.tar.gz
cp kuma-${KUMA_VER}/bin/kumactl ${REPO_ROOT}/bin/kumactl
chmod +x ${REPO_ROOT}/bin/kumactl

echo ">>> Installing Kuma ${KUMA_VER}"
${REPO_ROOT}/bin/kumactl install control-plane | kubectl apply -f -

echo ">>> Waiting for Kuma Control Plane to be ready"
kubectl wait --for condition=established crd/meshes.kuma.io
kubectl -n kuma-system rollout status deployment/kuma-control-plane

echo ">>> Configuring Default Kuma Mesh"
cat <<EOF | kubectl apply -f -
apiVersion: kuma.io/v1alpha1
kind: Mesh
metadata:
  name: default
spec:
  metrics:
    enabledBackend: prometheus-1
    backends:
      - name: prometheus-1
        type: prometheus
        conf:
          skipMTLS: true
          port: 5670
          path: /metrics
          tags:
            kuma.io/service: dataplane-metrics
  mtls:
    enabledBackend: ca-1
    backends:
      - name: ca-1
        type: builtin
        mode: PERMISSIVE
        dpCert:
          rotation:
            expiration: 1d
        conf:
          caCert:
            RSAbits: 2048
            expiration: 10y
EOF

echo '>>> Installing Kustomize'
cd ${REPO_ROOT}/bin && kustomize_url=https://github.com/kubernetes-sigs/kustomize/releases/download && \
curl -sL ${kustomize_url}/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_amd64.tar.gz | \
tar xz

echo '>>> Installing Flagger'
${REPO_ROOT}/bin/kustomize build ${REPO_ROOT}/test/kuma | kubectl apply -f -

kubectl -n kuma-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n kuma-system rollout status deployment/flagger
